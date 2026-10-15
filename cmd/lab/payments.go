package main

// Pembayaran (U2): daftar via Postgres `lab_ro` (paginasi kursor + filter),
// create diteruskan ke gateway (via Envoy), detail + timeline, cek status ke
// bank-sim. Semua uang integer IDR (P7/U6): tidak ada float di jalur ini.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ------------------------------------------------------------------ batas uang

const (
	// maxAmount = 1 triliun IDR (1e12). Jauh di bawah Number.MAX_SAFE_INTEGER
	// (2^53-1 ~ 9e15) dan BIGINT Postgres, sehingga aman di JS dan DB.
	maxAmount  = int64(1_000_000_000_000)
	maxIDLen   = 128
	defaultLim = 25
	maxLim     = 100
)

// ------------------------------------------------------------------ DTO (kontrak)

type paymentSummary struct {
	ID                string  `json:"id"`
	MerchantID        string  `json:"merchantId"`
	OrderID           string  `json:"orderId"`
	Channel           string  `json:"channel"`
	Provider          string  `json:"provider"`
	Amount            int64   `json:"amount"`
	Fee               int64   `json:"fee"`
	Status            string  `json:"status"`
	ProviderRef       *string `json:"providerRef"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	ExpiresAt         string  `json:"expiresAt"`
	CallbackCount     int     `json:"callbackCount"`
	WebhookDelivered  bool    `json:"webhookDelivered"`
	DuplicateCallback bool    `json:"duplicateCallback"`
}

type paymentEvent struct {
	Type       string  `json:"type"`
	At         *string `json:"at"`
	Derived    bool    `json:"derived"`
	Detail     *string `json:"detail"`
	Provider   *string `json:"provider"`
	Status     *string `json:"status"`
	Attempt    *int    `json:"attempt"`
	HTTPStatus *int    `json:"httpStatus"`
}

type paymentLinks struct {
	Log   *string `json:"log"`
	Trace *string `json:"trace"`
}

type paymentDetail struct {
	Payment           paymentSummary `json:"payment"`
	Timeline          []paymentEvent `json:"timeline"`
	CallbackCount     int            `json:"callbackCount"`
	WebhookCount      int            `json:"webhookCount"`
	DuplicateCallback bool           `json:"duplicateCallback"`
	TraceID           *string        `json:"traceId"`
	Links             paymentLinks   `json:"links"`
}

type paymentPage struct {
	CheckedAt  string           `json:"checkedAt"`
	DataStatus string           `json:"dataStatus"`
	HasMore    bool             `json:"hasMore"`
	NextCursor *string          `json:"nextCursor"`
	Items      []paymentSummary `json:"items"`
}

type bankStatus struct {
	Provider    string `json:"provider"`
	ProviderRef string `json:"providerRef"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	CheckedAt   string `json:"checkedAt"`
}

type createPaymentReq struct {
	MerchantID string      `json:"merchantId"`
	OrderID    string      `json:"orderId"`
	Amount     json.Number `json:"amount"`
	Channel    string      `json:"channel"`
}

type paymentFilter struct {
	Status   string
	Provider string
	Channel  string
	Cursor   string
	Limit    int
}

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Sentinel error.
var (
	errPaymentNotFound = errors.New("transaksi tidak ditemukan")
	errNoProviderRef   = errors.New("provider_ref belum ada (callback bank belum tiba)")
	errGatewayDown     = errors.New("gateway tidak terjangkau")
	errBankDown        = errors.New("bank-sim tidak terjangkau")
)

// ------------------------------------------------------------------ interface

type payments interface {
	List(ctx context.Context, f paymentFilter) (paymentPage, error)
	Get(ctx context.Context, id string) (paymentDetail, error)
	Create(ctx context.Context, req createPaymentReq) (paymentSummary, error)
	BankStatus(ctx context.Context, id string) (bankStatus, error)
}

// ------------------------------------------------------------------ config bank

type bankEndpoints struct {
	urls      map[string]string
	grafana   string
	loki      string
	tempo     string
	lokiName  string
	ingress   string
	adminURL  string
	userAgent string
}

func (b bankEndpoints) bankURL(provider string) (string, bool) {
	u, ok := b.urls[provider]
	return u, ok
}

// ------------------------------------------------------------------ store live (pgx)

type pgPayments struct {
	pool *poolCache
	dsn  string
	cfg  bankEndpoints
	http *http.Client
}

func newPgPayments(cfg bankEndpoints) *pgPayments {
	return &pgPayments{
		pool: newPoolCache(),
		dsn:  envOr("LAB_DATABASE_URL", defaultDatabaseURL),
		cfg:  cfg,
		http: &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *pgPayments) List(ctx context.Context, f paymentFilter) (paymentPage, error) {
	pool, err := p.pool.pool(p.dsn)
	if err != nil {
		return paymentPage{CheckedAt: rfc3339(time.Now()), DataStatus: "source_down", Items: []paymentSummary{}}, nil
	}
	qctx, cancel := queryCtx()
	defer cancel()
	items, next, more, err := listTransactions(qctx, pool, f)
	if err != nil {
		// DB tak terjangkau / error: jujur source_down, bukan 500.
		return paymentPage{CheckedAt: rfc3339(time.Now()), DataStatus: "source_down", Items: []paymentSummary{}}, nil
	}
	return paymentPage{
		CheckedAt:  rfc3339(time.Now()),
		DataStatus: "ok",
		HasMore:    more,
		NextCursor: next,
		Items:      items,
	}, nil
}

const txnCols = `t.id::text, t.merchant_id, t.order_id, t.channel, t.provider, t.amount, t.fee,
	t.status, t.provider_ref, t.created_at, t.updated_at, t.expires_at,
	(SELECT count(*) FROM callback_log c WHERE c.transaction_id = t.id) AS cb_count,
	EXISTS (SELECT 1 FROM webhook_delivery w WHERE w.transaction_id = t.id AND w.status = 'DELIVERED') AS webhook_ok`

func scanSummary(scan func(dest ...any) error) (paymentSummary, error) {
	var s paymentSummary
	var createdAt, updatedAt, expiresAt time.Time
	if err := scan(&s.ID, &s.MerchantID, &s.OrderID, &s.Channel, &s.Provider,
		&s.Amount, &s.Fee, &s.Status, &s.ProviderRef, &createdAt, &updatedAt, &expiresAt,
		&s.CallbackCount, &s.WebhookDelivered); err != nil {
		return paymentSummary{}, err
	}
	s.CreatedAt = rfc3339(createdAt)
	s.UpdatedAt = rfc3339(updatedAt)
	s.ExpiresAt = rfc3339(expiresAt)
	s.DuplicateCallback = s.CallbackCount > 1
	return s, nil
}

func listTransactions(ctx context.Context, pool *pgxpool.Pool, f paymentFilter) ([]paymentSummary, *string, bool, error) {
	limit := f.Limit
	if limit <= 0 || limit > maxLim {
		limit = defaultLim
	}
	where := []string{}
	args := []any{}
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	if f.Status != "" {
		where = append(where, "t.status = "+arg(f.Status))
	}
	if f.Provider != "" {
		where = append(where, "t.provider = "+arg(f.Provider))
	}
	if f.Channel != "" {
		where = append(where, "t.channel = "+arg(f.Channel))
	}
	// Keyset paginasi: (created_at, id) < (cursor).
	if f.Cursor != "" {
		cur, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, nil, false, err
		}
		where = append(where, "(t.created_at, t.id) < ("+arg(cur.createdAt)+"::timestamptz, "+arg(cur.id)+"::uuid)")
	}
	q := "SELECT " + txnCols + " FROM transactions t"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY t.created_at DESC, t.id DESC LIMIT " + arg(limit+1)

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, nil, false, err
	}
	defer rows.Close()
	items := []paymentSummary{}
	for rows.Next() {
		s, err := scanSummary(rows.Scan)
		if err != nil {
			return nil, nil, false, err
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	var next *string
	if more && len(items) > 0 {
		c := encodeCursor(items[len(items)-1].CreatedAt, items[len(items)-1].ID)
		next = &c
	}
	return items, next, more, nil
}

func (p *pgPayments) Get(ctx context.Context, id string) (paymentDetail, error) {
	pool, err := p.pool.pool(p.dsn)
	if err != nil {
		return paymentDetail{}, errGatewayDown
	}
	qctx, cancel := queryCtx()
	defer cancel()
	row := pool.QueryRow(qctx, "SELECT "+txnCols+" FROM transactions t WHERE t.id = $1::uuid", id)
	sum, err := scanSummary(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return paymentDetail{}, errPaymentNotFound
		}
		return paymentDetail{}, err
	}
	callbacks, err := p.callbacks(qctx, pool, id)
	if err != nil {
		return paymentDetail{}, err
	}
	webhooks, err := p.webhooks(qctx, pool, id)
	if err != nil {
		return paymentDetail{}, err
	}
	detail := buildDetail(sum, callbacks, webhooks)
	detail.Links = p.obsLinks(sum.ID)
	return detail, nil
}

type callbackRow struct {
	provider   string
	status     string
	receivedAt time.Time
}

func (p *pgPayments) callbacks(ctx context.Context, pool *pgxpool.Pool, id string) ([]callbackRow, error) {
	rows, err := pool.Query(ctx,
		`SELECT provider, coalesce(payload->>'status',''), received_at
		 FROM callback_log WHERE transaction_id = $1::uuid ORDER BY received_at, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []callbackRow
	for rows.Next() {
		var c callbackRow
		if err := rows.Scan(&c.provider, &c.status, &c.receivedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type webhookRow struct {
	attempt    int
	httpStatus *int
	status     string
	createdAt  time.Time
}

func (p *pgPayments) webhooks(ctx context.Context, pool *pgxpool.Pool, id string) ([]webhookRow, error) {
	rows, err := pool.Query(ctx,
		`SELECT attempt, http_status, status, created_at
		 FROM webhook_delivery WHERE transaction_id = $1::uuid ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []webhookRow
	for rows.Next() {
		var w webhookRow
		if err := rows.Scan(&w.attempt, &w.httpStatus, &w.status, &w.createdAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// buildDetail menyusun timeline dari waktu NYATA (created_at, callback_log.
// received_at, webhook_delivery.created_at, updated_at). Langkah CREATED->PENDING
// tidak punya kolom waktu sendiri di DB (P8 hanya menyimpan status sekarang),
// jadi ditandai `derived=true` tanpa timestamp — bukan waktu yang dikarang.
func buildDetail(sum paymentSummary, callbacks []callbackRow, webhooks []webhookRow) paymentDetail {
	events := make([]paymentEvent, 0, 3+len(callbacks)+len(webhooks))
	created := sum.CreatedAt
	events = append(events, paymentEvent{Type: "created", At: &created, Derived: false})
	if sum.Status != "CREATED" {
		events = append(events, paymentEvent{Type: "pending", At: nil, Derived: true,
			Detail: strptr("diterima gateway, menunggu callback/expiry")})
	}
	for _, c := range callbacks {
		at := rfc3339(c.receivedAt)
		prov := c.provider
		var st *string
		if c.status != "" {
			st = &c.status
		}
		events = append(events, paymentEvent{Type: "callback", At: &at, Derived: false, Provider: &prov, Status: st})
	}
	if sum.Status == "SUCCESS" || sum.Status == "FAILED" || sum.Status == "EXPIRED" {
		at := sum.UpdatedAt
		st := sum.Status
		events = append(events, paymentEvent{Type: "final", At: &at, Derived: false, Status: &st})
	}
	for _, w := range webhooks {
		at := rfc3339(w.createdAt)
		attempt := w.attempt
		detail := "webhook " + w.status
		events = append(events, paymentEvent{
			Type: "webhook", At: &at, Derived: false, Detail: &detail,
			Attempt: &attempt, HTTPStatus: w.httpStatus,
		})
	}
	return paymentDetail{
		Payment:           sum,
		Timeline:          events,
		CallbackCount:     len(callbacks),
		WebhookCount:      len(webhooks),
		DuplicateCallback: len(callbacks) > 1,
		TraceID:           nil, // gateway belum menyimpan traceparent (lihat ASSUMPTIONS U2)
		Links:             paymentLinks{},
	}
}

// obsLinks: tautan Grafana Explore bila obs-full aktif (env LAB_LOKI_URL/
// LAB_TEMPO_URL). Bila tidak, null (fitur belum aktif, bukan error).
func (p *pgPayments) obsLinks(txnID string) paymentLinks {
	var links paymentLinks
	if p.cfg.loki != "" {
		expr := `{service="gateway-api"} |= "` + txnID + `"`
		links.Log = strptr(exploreLink(p.cfg.grafana, p.cfg.lokiName, "logs", expr))
	}
	// traceId tidak tersimpan di DB (U2), jadi tautan trace tidak dibuat.
	return links
}

func (p *pgPayments) BankStatus(ctx context.Context, id string) (bankStatus, error) {
	pool, err := p.pool.pool(p.dsn)
	if err != nil {
		return bankStatus{}, errGatewayDown
	}
	qctx, cancel := queryCtx()
	defer cancel()
	var provider string
	var providerRef *string
	if err := pool.QueryRow(qctx,
		`SELECT provider, provider_ref FROM transactions WHERE id = $1::uuid`, id).
		Scan(&provider, &providerRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return bankStatus{}, errPaymentNotFound
		}
		return bankStatus{}, err
	}
	if providerRef == nil || *providerRef == "" {
		return bankStatus{}, errNoProviderRef
	}
	base, ok := p.cfg.bankURL(provider)
	if !ok {
		return bankStatus{}, fmt.Errorf("%w: provider %q", errBankDown, provider)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/status/"+*providerRef, nil)
	if err != nil {
		return bankStatus{}, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return bankStatus{}, errBankDown
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == http.StatusNotFound {
		return bankStatus{
			Provider: provider, ProviderRef: *providerRef, Status: "UNKNOWN",
			Source: "bank", CheckedAt: rfc3339(time.Now()),
		}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return bankStatus{}, errBankDown
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return bankStatus{}, errBankDown
	}
	st := strings.ToUpper(out.Status)
	switch st {
	case "CHARGED", "SUCCESS", "FAILED":
	default:
		st = "UNKNOWN"
	}
	return bankStatus{
		Provider: provider, ProviderRef: *providerRef, Status: st,
		Source: "bank", CheckedAt: rfc3339(time.Now()),
	}, nil
}

func (p *pgPayments) Create(ctx context.Context, req createPaymentReq) (paymentSummary, error) {
	payload := map[string]any{
		"merchant_id": strings.TrimSpace(req.MerchantID),
		"order_id":    strings.TrimSpace(req.OrderID),
		"amount":      mustInt64(req.Amount),
		"channel":     req.Channel,
	}
	body, _ := json.Marshal(payload)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.ingress+"/v1/payments", strings.NewReader(string(body)))
	if err != nil {
		return paymentSummary{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(hreq)
	if err != nil {
		return paymentSummary{}, errGatewayDown
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return paymentSummary{}, fmt.Errorf("%w (HTTP %d): %s", errGatewayDown, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var gw struct {
		ID          string  `json:"id"`
		MerchantID  string  `json:"merchant_id"`
		OrderID     string  `json:"order_id"`
		Channel     string  `json:"channel"`
		Provider    string  `json:"provider"`
		Amount      int64   `json:"amount"`
		Fee         int64   `json:"fee"`
		Status      string  `json:"status"`
		ProviderRef *string `json:"provider_ref"`
		CreatedAt   string  `json:"created_at"`
		ExpiresAt   string  `json:"expires_at"`
	}
	if err := json.Unmarshal(respBody, &gw); err != nil {
		return paymentSummary{}, fmt.Errorf("%w: respons gateway tak terbaca", errGatewayDown)
	}
	return paymentSummary{
		ID: gw.ID, MerchantID: gw.MerchantID, OrderID: gw.OrderID,
		Channel: gw.Channel, Provider: gw.Provider, Amount: gw.Amount, Fee: gw.Fee,
		Status: gw.Status, ProviderRef: gw.ProviderRef,
		CreatedAt: normalizeTS(gw.CreatedAt), UpdatedAt: normalizeTS(gw.CreatedAt),
		ExpiresAt: normalizeTS(gw.ExpiresAt),
	}, nil
}

// normalizeTS menormalkan timestamp gateway (Postgres timestamptz `::text`,
// mis. `2026-10-15 11:46:46.918291+00`) ke RFC3339 UTC detik — sama dengan
// format list/detail. Bila tak terbaca, dikembalikan apa adanya.
func normalizeTS(s string) string {
	if s == "" {
		return s
	}
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999-07",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return rfc3339(t)
		}
	}
	return s
}

func mustInt64(n json.Number) int64 {
	v, err := n.Int64()
	if err != nil {
		return 0
	}
	return v
}

// ------------------------------------------------------------------ validasi

// validateCreate: pesan jelas per-field (dipakai handler + test).
func validateCreate(req createPaymentReq) []fieldError {
	var errs []fieldError
	if strings.TrimSpace(req.MerchantID) == "" {
		errs = append(errs, fieldError{"merchantId", "merchantId wajib diisi"})
	} else if len(req.MerchantID) > maxIDLen {
		errs = append(errs, fieldError{"merchantId", fmt.Sprintf("merchantId maksimal %d karakter", maxIDLen)})
	}
	if strings.TrimSpace(req.OrderID) == "" {
		errs = append(errs, fieldError{"orderId", "orderId wajib diisi"})
	} else if len(req.OrderID) > maxIDLen {
		errs = append(errs, fieldError{"orderId", fmt.Sprintf("orderId maksimal %d karakter", maxIDLen)})
	}
	amount, err := req.Amount.Int64()
	switch {
	case err != nil:
		errs = append(errs, fieldError{"amount", "amount harus integer IDR (tanpa pecahan/eksponen)"})
	case amount <= 0:
		errs = append(errs, fieldError{"amount", "amount harus lebih besar dari 0"})
	case amount > maxAmount:
		errs = append(errs, fieldError{"amount", fmt.Sprintf("amount maksimal %d IDR", maxAmount)})
	}
	switch req.Channel {
	case "VA", "EWALLET", "QRIS":
	default:
		errs = append(errs, fieldError{"channel", "channel harus VA, EWALLET, atau QRIS"})
	}
	return errs
}

// ------------------------------------------------------------------ kursor

type cursor struct {
	createdAt time.Time
	id        string
}

func encodeCursor(createdAt, id string) string {
	raw := createdAt + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, fmt.Errorf("kursor tidak valid")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return cursor{}, fmt.Errorf("kursor tidak valid")
	}
	ts, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		ts, err = time.Parse(time.RFC3339, parts[0])
		if err != nil {
			return cursor{}, fmt.Errorf("kursor tidak valid")
		}
	}
	if parts[1] == "" {
		return cursor{}, fmt.Errorf("kursor tidak valid")
	}
	return cursor{createdAt: ts, id: parts[1]}, nil
}

// exploreLink membuat deep-link Grafana Explore (best-effort; lihat ASSUMPTIONS).
func exploreLink(grafana, datasource, kind, expr string) string {
	if grafana == "" {
		grafana = "http://127.0.0.1:3002"
	}
	left := map[string]any{
		"datasource": datasource,
		"range":      map[string]string{"from": "now-1h", "to": "now"},
	}
	if kind == "traces" {
		left["queries"] = []map[string]any{{"refId": "A", "queryType": "traceql", "query": expr}}
	} else {
		left["queries"] = []map[string]any{{"refId": "A", "expr": expr}}
	}
	enc, _ := json.Marshal(left)
	return grafana + "/explore?left=" + url.QueryEscape(string(enc))
}
