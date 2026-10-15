package main

// Test U2: validasi amount, paginasi kursor, timeline, kontrol + audit,
// read-only lab_ro (integrasi, dilewati bila DB mati).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func doJSON(t *testing.T, h http.Handler, method, target, body, actor string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = "127.0.0.1:8088"
	req.Header.Set("Content-Type", "application/json")
	if method != "GET" {
		req.Header.Set("Origin", "http://127.0.0.1:8088")
		req.Header.Set("X-Lab-Token", testToken)
	}
	if actor != "" {
		req.Header.Set("X-Lab-Actor", actor)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) (string, []fieldError) {
	t.Helper()
	var p struct {
		Detail string       `json:"detail"`
		Errors []fieldError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("problem bukan JSON: %v (%s)", err, rec.Body.String())
	}
	return p.Detail, p.Errors
}

// ---------- validasi amount (acceptance: negatif/pecahan/di luar batas) ----------

func TestValidateCreateAmount(t *testing.T) {
	cases := []struct {
		name    string
		amount  string
		wantErr bool
		field   string
	}{
		{"negatif", "-5", true, "amount"},
		{"nol", "0", true, "amount"},
		{"pecahan", "1500.5", true, "amount"},
		{"eksponen", "1e3", true, "amount"},
		{"melebihi batas", "1000000000001", true, "amount"},
		{"batas atas ok", "1000000000000", false, ""},
		{"minimum ok", "1", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer()
			body := `{"merchantId":"m","orderId":"o","amount":` + c.amount + `,"channel":"VA"}`
			rec := doJSON(t, s.handler(), "POST", "/api/v1/payments", body, "trainee")
			if c.wantErr {
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("got %d, want 400 (%s)", rec.Code, rec.Body.String())
				}
				detail, errs := decodeProblem(t, rec)
				if detail == "" || len(errs) == 0 {
					t.Fatalf("pesan validasi tidak jelas: %s", rec.Body.String())
				}
				if errs[0].Field != c.field {
					t.Fatalf("field %q, want %q", errs[0].Field, c.field)
				}
			} else if rec.Code != http.StatusCreated {
				t.Fatalf("got %d, want 201 (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateRejectsUnknownField(t *testing.T) {
	s := testServer()
	body := `{"merchantId":"m","orderId":"o","amount":1000,"channel":"VA","extra":1}`
	rec := doJSON(t, s.handler(), "POST", "/api/v1/payments", body, "trainee")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestCreateChannelEnum(t *testing.T) {
	s := testServer()
	body := `{"merchantId":"m","orderId":"o","amount":1000,"channel":"BOGUS"}`
	rec := doJSON(t, s.handler(), "POST", "/api/v1/payments", body, "trainee")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	_, errs := decodeProblem(t, rec)
	if len(errs) == 0 || errs[0].Field != "channel" {
		t.Fatalf("error channel tidak dilaporkan: %s", rec.Body.String())
	}
}

// ---------- paginasi kursor ----------

func TestPaymentsListDemoCursor(t *testing.T) {
	s := testServer()
	seen := map[string]bool{}
	cursor := ""
	total := 0
	for page := 0; page < 10; page++ {
		target := "/api/v1/payments?limit=2"
		if cursor != "" {
			target += "&cursor=" + cursor
		}
		rec := doJSON(t, s.handler(), "GET", target, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("page %d: got %d (%s)", page, rec.Code, rec.Body.String())
		}
		var out paymentPage
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("bukan JSON: %v", err)
		}
		if out.DataStatus != "ok" {
			t.Fatalf("dataStatus %q", out.DataStatus)
		}
		for _, it := range out.Items {
			if seen[it.ID] {
				t.Fatalf("duplikat %s", it.ID)
			}
			seen[it.ID] = true
			total++
		}
		if !out.HasMore {
			if out.NextCursor != nil {
				t.Fatal("hasMore=false tapi nextCursor ada")
			}
			break
		}
		if out.NextCursor == nil {
			t.Fatal("hasMore=true tapi nextCursor nil")
		}
		cursor = *out.NextCursor
	}
	if total != 5 {
		t.Fatalf("total %d, want 5", total)
	}
}

func TestPaymentsListFilterAndBadCursor(t *testing.T) {
	s := testServer()
	rec := doJSON(t, s.handler(), "GET", "/api/v1/payments?provider=bca", "", "")
	var out paymentPage
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	for _, it := range out.Items {
		if it.Provider != "bca" {
			t.Fatalf("filter provider bocor: %s", it.Provider)
		}
	}
	// Filter tak dikenal -> 400.
	rec = doJSON(t, s.handler(), "GET", "/api/v1/payments?status=NOPE", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status buruk: got %d", rec.Code)
	}
	// Kursor rusak -> 400.
	rec = doJSON(t, s.handler(), "GET", "/api/v1/payments?cursor=zzzz", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("kursor buruk: got %d", rec.Code)
	}
}

// ---------- timeline & duplikat ----------

func TestPaymentDetailTimelineDuplicate(t *testing.T) {
	s := testServer()
	id := "018f0a2e-1c3d-7a4b-8e5f-000000000004" // fixture: 2 callback
	rec := doJSON(t, s.handler(), "GET", "/api/v1/payments/"+id, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
	var d paymentDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("bukan JSON: %v", err)
	}
	if d.CallbackCount != 2 || !d.DuplicateCallback {
		t.Fatalf("callbackCount=%d dup=%v, want 2/true", d.CallbackCount, d.DuplicateCallback)
	}
	if d.WebhookCount != 2 {
		t.Fatalf("webhookCount=%d, want 2", d.WebhookCount)
	}
	kinds := map[string]int{}
	for _, e := range d.Timeline {
		kinds[e.Type]++
		if e.Type == "pending" && (e.At != nil || !e.Derived) {
			t.Fatal("pending harus derived tanpa timestamp")
		}
		if e.Type != "pending" && e.At == nil {
			t.Fatalf("event %s tanpa timestamp", e.Type)
		}
	}
	for _, want := range []string{"created", "pending", "callback", "final", "webhook"} {
		if kinds[want] == 0 {
			t.Fatalf("timeline tanpa %q: %+v", want, kinds)
		}
	}
	if kinds["callback"] != 2 {
		t.Fatalf("callback events %d, want 2", kinds["callback"])
	}
}

func TestPaymentDetailNotFound(t *testing.T) {
	s := testServer()
	for _, id := range []string{"bukan-uuid", "018f0a2e-1c3d-7a4b-8e5f-0000000000ff"} {
		rec := doJSON(t, s.handler(), "GET", "/api/v1/payments/"+id, "", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("id %q: got %d, want 404", id, rec.Code)
		}
	}
}

func TestBankStatusDemo(t *testing.T) {
	s := testServer()
	// SUCCESS txn.
	rec := doJSON(t, s.handler(), "GET", "/api/v1/payments/018f0a2e-1c3d-7a4b-8e5f-000000000002/bank-status", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d (%s)", rec.Code, rec.Body.String())
	}
	var bs bankStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &bs)
	if bs.Status != "SUCCESS" || bs.Source != "bank" || bs.ProviderRef == "" {
		t.Fatalf("bank status aneh: %+v", bs)
	}
	// PENDING -> CHARGED.
	rec = doJSON(t, s.handler(), "GET", "/api/v1/payments/018f0a2e-1c3d-7a4b-8e5f-000000000001/bank-status", "", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &bs)
	if bs.Status != "CHARGED" {
		t.Fatalf("pending bank status %q, want CHARGED", bs.Status)
	}
}

// ---------- kontrol + audit ----------

func TestRoutingPutAuditAndValidation(t *testing.T) {
	s := testServer()
	// Disable bca (mitigasi skenario 01).
	body := `{"bca":{"enabled":false,"weight":60},"mandiri":{"enabled":true,"weight":40},"ewallet":{"enabled":true,"weight":100}}`
	rec := doJSON(t, s.handler(), "PUT", "/api/v1/routing", body, "sinta")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d (%s)", rec.Code, rec.Body.String())
	}
	var rc routingConfig
	_ = json.Unmarshal(rec.Body.Bytes(), &rc)
	if rc.Bca.Enabled {
		t.Fatal("bca masih enabled")
	}
	// GET mengembalikan yang tersimpan.
	rec = doJSON(t, s.handler(), "GET", "/api/v1/routing", "", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &rc)
	if rc.Bca.Enabled {
		t.Fatal("GET routing tidak konsisten")
	}
	// Audit mencatat aktor + aksi + target + waktu.
	rec = doJSON(t, s.handler(), "GET", "/api/v1/audit", "", "")
	var al struct {
		Entries []auditEntry `json:"entries"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &al)
	if len(al.Entries) == 0 {
		t.Fatal("audit kosong")
	}
	e := al.Entries[0]
	if e.Actor != "sinta" || e.Action != "routing.set" || e.Target != "gateway" {
		t.Fatalf("audit entry salah: %+v", e)
	}
	if _, err := time.Parse(time.RFC3339, e.TS); err != nil {
		t.Fatalf("audit ts bukan RFC3339: %q", e.TS)
	}
	// Weight di luar 0..100 -> 400.
	bad := `{"bca":{"enabled":true,"weight":101},"mandiri":{"enabled":true,"weight":40},"ewallet":{"enabled":true,"weight":100}}`
	rec = doJSON(t, s.handler(), "PUT", "/api/v1/routing", bad, "sinta")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("weight 101: got %d, want 400", rec.Code)
	}
}

func TestTrafficPutValidationAndAudit(t *testing.T) {
	s := testServer()
	for _, rate := range []string{"0", "101", "1.5", "-3"} {
		body := `{"ratePerSec":` + rate + `}`
		rec := doJSON(t, s.handler(), "PUT", "/api/v1/traffic", body, "sinta")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("rate %s: got %d, want 400", rate, rec.Code)
		}
	}
	rec := doJSON(t, s.handler(), "PUT", "/api/v1/traffic", `{"ratePerSec":5}`, "sinta")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d (%s)", rec.Code, rec.Body.String())
	}
	var ts trafficState
	_ = json.Unmarshal(rec.Body.Bytes(), &ts)
	if ts.RatePerSec == nil || *ts.RatePerSec != 5 || ts.Source != "loadgen" {
		t.Fatalf("traffic state: %+v", ts)
	}
}

func TestReleasePutValidation(t *testing.T) {
	s := testServer()
	rec := doJSON(t, s.handler(), "PUT", "/api/v1/release", `{"version":"bogus"}`, "sinta")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	rec = doJSON(t, s.handler(), "PUT", "/api/v1/release", `{"version":"bad"}`, "sinta")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d (%s)", rec.Code, rec.Body.String())
	}
	var e auditEntry
	rec = doJSON(t, s.handler(), "GET", "/api/v1/audit?limit=1", "", "")
	var al struct {
		Entries []auditEntry `json:"entries"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &al)
	e = al.Entries[0]
	if e.Action != "release.set" || e.Detail != "version=bad" {
		t.Fatalf("audit release: %+v", e)
	}
}

func TestMutationRequiresTokenNoAudit(t *testing.T) {
	s := testServer()
	req := httptest.NewRequest("PUT", "/api/v1/release", strings.NewReader(`{"version":"bad"}`))
	req.Host = "127.0.0.1:8088"
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
	rec = doJSON(t, s.handler(), "GET", "/api/v1/audit", "", "")
	var al struct {
		Entries []auditEntry `json:"entries"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &al)
	if len(al.Entries) != 0 {
		t.Fatalf("audit terisi walau request ditolak: %+v", al.Entries)
	}
}

// ---------- integrasi: lab_ro read-only (acceptance) ----------

// TestLiveLabROReadOnly membuktikan role lab_ro tidak bisa memutasi data.
// Dilewati bila Postgres tak terjangkau (mis. CI tanpa stack).
func TestLiveLabROReadOnly(t *testing.T) {
	dsn := envOr("LAB_TEST_RO_DSN", defaultDatabaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip: pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skip: Postgres tak terjangkau: %v", err)
	}
	// SELECT boleh.
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions").Scan(&n); err != nil {
		t.Fatalf("SELECT gagal untuk lab_ro: %v", err)
	}
	// Mutasi wajib ditolak (permission denied).
	mutations := []string{
		`INSERT INTO transactions (id, merchant_id, order_id, channel, provider, amount, fee, status, expires_at)
		 VALUES (gen_random_uuid(), 'x', 'ro-probe', 'VA', 'bca', 1, 0, 'PENDING', now())`,
		`UPDATE transactions SET status='FAILED' WHERE order_id='ro-probe'`,
		`DELETE FROM transactions WHERE order_id='ro-probe'`,
	}
	for _, q := range mutations {
		_, err := pool.Exec(ctx, q)
		if err == nil {
			t.Fatalf("MUTASI DIIZINKAN untuk lab_ro (harus gagal): %s", q)
		}
	}
}

// ---------- uji live opsional ke gateway/Envoy (LAB_TEST_LIVE=1) ----------

func TestLiveGatewayRoutingRoundtrip(t *testing.T) {
	if os.Getenv("LAB_TEST_LIVE") == "" {
		t.Skip("set LAB_TEST_LIVE=1 untuk uji gateway hidup")
	}
	live := newServer(serverConfig{demo: false, webDir: "testdata/web", token: testToken, port: testPort,
		repoRoot: t.TempDir()})
	rec := doJSON(t, live.handler(), "GET", "/api/v1/routing", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("routing live: got %d (%s)", rec.Code, rec.Body.String())
	}
}
