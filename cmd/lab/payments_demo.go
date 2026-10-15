package main

// Fixture pembayaran untuk mode --demo (deterministik, tanpa Docker/DB).
// Dipakai test Go, Vitest, dan Playwright e2e (tanpa stack hidup).

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type demoTxn struct {
	summary   paymentSummary
	callbacks []callbackRow
	webhooks  []webhookRow
}

type demoPayments struct {
	mu   sync.Mutex
	txns []demoTxn
	seq  int
}

func demoTS(secondsAgo int) string {
	return rfc3339(demoTime().Add(-time.Duration(secondsAgo) * time.Second))
}

func demoTSAt(secondsAgo int) time.Time {
	return demoTime().Add(-time.Duration(secondsAgo) * time.Second)
}

func ref(s string) *string { return &s }

func newDemoPayments() *demoPayments {
	d := &demoPayments{}
	d.txns = []demoTxn{
		{
			summary: paymentSummary{
				ID: "018f0a2e-1c3d-7a4b-8e5f-000000000001", MerchantID: "merchant-demo",
				OrderID: "demo-order-001", Channel: "VA", Provider: "bca", Amount: 150_000,
				Fee: 4000, Status: "PENDING", ProviderRef: ref("P-BCA-0001"),
				CreatedAt: demoTS(300), UpdatedAt: demoTS(299), ExpiresAt: demoTS(-300),
			},
		},
		{
			summary: paymentSummary{
				ID: "018f0a2e-1c3d-7a4b-8e5f-000000000002", MerchantID: "merchant-demo",
				OrderID: "demo-order-002", Channel: "VA", Provider: "mandiri", Amount: 1_250_000,
				Fee: 4000, Status: "SUCCESS", ProviderRef: ref("P-MANDIRI-0002"),
				CreatedAt: demoTS(240), UpdatedAt: demoTS(230), ExpiresAt: demoTS(-360),
				CallbackCount: 1, WebhookDelivered: true,
			},
			callbacks: []callbackRow{{provider: "mandiri", status: "SUCCESS", receivedAt: demoTSAt(231)}},
			webhooks:  []webhookRow{{attempt: 1, httpStatus: intp(200), status: "DELIVERED", createdAt: demoTSAt(230)}},
		},
		{
			summary: paymentSummary{
				ID: "018f0a2e-1c3d-7a4b-8e5f-000000000003", MerchantID: "merchant-demo",
				OrderID: "demo-order-003", Channel: "EWALLET", Provider: "ewallet", Amount: 99_000,
				Fee: 2000, Status: "FAILED", ProviderRef: ref("P-EWALLET-0003"),
				CreatedAt: demoTS(180), UpdatedAt: demoTS(170), ExpiresAt: demoTS(-420),
				CallbackCount: 1,
			},
			callbacks: []callbackRow{{provider: "ewallet", status: "FAILED", receivedAt: demoTSAt(171)}},
			webhooks:  []webhookRow{{attempt: 1, httpStatus: intp(500), status: "FAILED", createdAt: demoTSAt(170)}},
		},
		{
			summary: paymentSummary{
				ID: "018f0a2e-1c3d-7a4b-8e5f-000000000004", MerchantID: "merchant-demo",
				OrderID: "demo-order-004", Channel: "VA", Provider: "bca", Amount: 2_000_000,
				Fee: 4000, Status: "SUCCESS", ProviderRef: ref("P-BCA-0004"),
				CreatedAt: demoTS(120), UpdatedAt: demoTS(100), ExpiresAt: demoTS(-480),
				CallbackCount: 2, WebhookDelivered: true, DuplicateCallback: true,
			},
			callbacks: []callbackRow{
				{provider: "bca", status: "SUCCESS", receivedAt: demoTSAt(110)},
				{provider: "bca", status: "SUCCESS", receivedAt: demoTSAt(109)},
			},
			webhooks: []webhookRow{
				{attempt: 1, httpStatus: intp(200), status: "DELIVERED", createdAt: demoTSAt(109)},
				{attempt: 1, httpStatus: intp(200), status: "DELIVERED", createdAt: demoTSAt(100)},
			},
		},
		{
			summary: paymentSummary{
				ID: "018f0a2e-1c3d-7a4b-8e5f-000000000005", MerchantID: "merchant-demo",
				OrderID: "demo-order-005", Channel: "QRIS", Provider: "ewallet", Amount: 45_000,
				Fee: 1500, Status: "EXPIRED", ProviderRef: ref("P-EWALLET-0005"),
				CreatedAt: demoTS(60), UpdatedAt: demoTS(30), ExpiresAt: demoTS(30),
			},
		},
	}
	d.seq = 5
	return d
}

func intp(v int) *int { return &v }

func (d *demoPayments) List(_ context.Context, f paymentFilter) (paymentPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	filtered := make([]paymentSummary, 0, len(d.txns))
	for _, t := range d.txns {
		s := t.summary
		if f.Status != "" && s.Status != f.Status {
			continue
		}
		if f.Provider != "" && s.Provider != f.Provider {
			continue
		}
		if f.Channel != "" && s.Channel != f.Channel {
			continue
		}
		filtered = append(filtered, s)
	}
	limit := f.Limit
	if limit <= 0 || limit > maxLim {
		limit = defaultLim
	}
	start := 0
	if f.Cursor != "" {
		idx, err := decodeDemoCursor(f.Cursor)
		if err != nil {
			return paymentPage{}, err
		}
		start = idx
	}
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	items := filtered[start:end]
	more := end < len(filtered)
	var next *string
	if more {
		c := encodeDemoCursor(end)
		next = &c
	}
	return paymentPage{
		CheckedAt: demoBase, DataStatus: "ok", HasMore: more,
		NextCursor: next, Items: items,
	}, nil
}

func (d *demoPayments) Get(_ context.Context, id string) (paymentDetail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.txns {
		if t.summary.ID == id {
			detail := buildDetail(t.summary, t.callbacks, t.webhooks)
			return detail, nil
		}
	}
	return paymentDetail{}, errPaymentNotFound
}

func (d *demoPayments) BankStatus(_ context.Context, id string) (bankStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, t := range d.txns {
		if t.summary.ID != id {
			continue
		}
		if t.summary.ProviderRef == nil {
			return bankStatus{}, errNoProviderRef
		}
		st := "CHARGED"
		switch t.summary.Status {
		case "SUCCESS":
			st = "SUCCESS"
		case "FAILED":
			st = "FAILED"
		}
		return bankStatus{
			Provider: t.summary.Provider, ProviderRef: *t.summary.ProviderRef,
			Status: st, Source: "bank", CheckedAt: demoBase,
		}, nil
	}
	return bankStatus{}, errPaymentNotFound
}

func (d *demoPayments) Create(_ context.Context, req createPaymentReq) (paymentSummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seq++
	provider := "ewallet"
	if req.Channel == "VA" {
		provider = "bca"
	}
	id := fmt.Sprintf("018f0a2e-1c3d-7a4b-8e5f-%012d", d.seq)
	s := paymentSummary{
		ID: id, MerchantID: strings.TrimSpace(req.MerchantID), OrderID: strings.TrimSpace(req.OrderID),
		Channel: req.Channel, Provider: provider, Amount: mustInt64(req.Amount),
		Fee: 4000, Status: "PENDING", ProviderRef: ref("P-DEMO-" + strconv.Itoa(d.seq)),
		CreatedAt: demoBase, UpdatedAt: demoBase, ExpiresAt: demoBase,
	}
	d.txns = append([]demoTxn{{summary: s}}, d.txns...)
	return s, nil
}

func encodeDemoCursor(idx int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("d|" + strconv.Itoa(idx)))
}

func decodeDemoCursor(s string) (int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("kursor tidak valid")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 || parts[0] != "d" {
		return 0, fmt.Errorf("kursor tidak valid")
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("kursor tidak valid")
	}
	return n, nil
}
