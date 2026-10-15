package main

// Metrik bernama untuk Mission Control (U1).
//
// Browser TIDAK PERNAH mengirim PromQL bebas: Go memetakan nama allowlist
// ke query tetap (parameter window/provider/step divalidasi ketat).
// Semantik data (UI-SPEC): ok | no_data | stale | source_down.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Batas keras U1.
const (
	promTimeout     = 5 * time.Second
	promRangeTimout = 10 * time.Second
	staleAfter      = 15 * time.Second
	maxSeriesPoints = 1000
)

var validWindows = map[string]bool{"1m": true, "5m": true, "15m": true, "1h": true}
var validSteps = map[string]bool{"5s": true, "15s": true, "1m": true}
var validProviders = map[string]bool{"bca": true, "mandiri": true, "ewallet": true}
var validDemos = map[string]bool{"healthy": true, "bca_down": true, "prom_down": true, "empty": true, "stale": true, "payments": true}

func windowSeconds(w string) int64 {
	switch w {
	case "1m":
		return 60
	case "5m":
		return 300
	case "15m":
		return 900
	case "1h":
		return 3600
	}
	return 300
}

func stepSeconds(s string) int64 {
	switch s {
	case "5s":
		return 5
	case "15s":
		return 15
	case "1m":
		return 60
	}
	return 15
}

type metricValue struct {
	Status     string   `json:"status"`
	Value      *float64 `json:"value"`
	SampleRate *float64 `json:"sampleRate"`
	LastDataAt *string  `json:"lastDataAt"`
	Detail     string   `json:"detail,omitempty"`
}

type promSample struct {
	metric map[string]string
	value  float64
	ts     time.Time
}

// Query bernama. provider hanya dipakai bila needsProvider.
type namedQuery struct {
	needsProvider bool
	build         func(window, provider string) string
	// rateQuery opsional untuk sampleRate (nil = tanpa laju).
	rate func(window, provider string) string
}

const ingressSel = `envoy_http_conn_manager_prefix="ingress"`

func rqClass(class, window string) string {
	return fmt.Sprintf(`sum(rate(envoy_http_downstream_rq_xx{%s,envoy_response_code_class="%s"}[%s]))`, ingressSel, class, window)
}

func rqAll(window string) string {
	return fmt.Sprintf(`sum(rate(envoy_http_downstream_rq_xx{%s}[%s]))`, ingressSel, window)
}

func payTotal(window, provider string) string {
	sel := ""
	if provider != "" {
		sel = fmt.Sprintf(`provider="%s",`, provider)
	}
	return fmt.Sprintf(`sum(rate(payment_transactions_total{%s}[%s]))`, trimTrailingComma(sel), window)
}

func trimTrailingComma(s string) string {
	return strings.TrimSuffix(s, ",")
}

var metricQueries = map[string]namedQuery{
	"http_rps": {build: func(w, _ string) string {
		return fmt.Sprintf(`sum(rate(envoy_http_downstream_rq_total{%s}[%s]))`, ingressSel, w)
	}},
	"http_2xx_ratio": {build: func(w, _ string) string {
		return rqClass("2", w) + " / " + rqAll(w)
	}, rate: func(w, _ string) string {
		return fmt.Sprintf(`sum(rate(envoy_http_downstream_rq_total{%s}[%s]))`, ingressSel, w)
	}},
	"http_5xx_ratio": {build: func(w, _ string) string {
		return rqClass("5", w) + " / " + rqAll(w)
	}, rate: func(w, _ string) string {
		return fmt.Sprintf(`sum(rate(envoy_http_downstream_rq_total{%s}[%s]))`, ingressSel, w)
	}},
	"http_p99_ms": {build: func(w, _ string) string {
		return fmt.Sprintf(`histogram_quantile(0.99, sum by (le) (rate(envoy_http_downstream_rq_time_bucket{%s}[%s])))`, ingressSel, w)
	}, rate: func(w, _ string) string {
		return fmt.Sprintf(`sum(rate(envoy_http_downstream_rq_total{%s}[%s]))`, ingressSel, w)
	}},
	"payment_rps": {build: func(w, _ string) string { return payTotal(w, "") }},
	"payment_success_ratio": {build: func(w, _ string) string {
		return fmt.Sprintf(`sum(rate(payment_transactions_total{status="SUCCESS"}[%s])) / %s`, w, payTotal(w, ""))
	}, rate: func(w, _ string) string { return payTotal(w, "") }},
	"payment_success_ratio_by_provider": {needsProvider: true, build: func(w, p string) string {
		return fmt.Sprintf(`sum(rate(payment_transactions_total{provider="%s",status="SUCCESS"}[%s])) / %s`, p, w, payTotal(w, p))
	}, rate: func(w, p string) string { return payTotal(w, p) }},
	"payment_pending_stuck": {build: func(_, _ string) string {
		return `payment_transactions_pending`
	}},
	"webhook_delivery_ratio": {build: func(w, _ string) string {
		return fmt.Sprintf(`sum(rate(payment_webhook_deliveries_total{status="DELIVERED"}[%s])) / sum(rate(payment_webhook_deliveries_total[%s]))`, w, w)
	}, rate: func(w, _ string) string { return payTotal(w, "") }},
}

// Urutan stabil untuk summary.
var summaryMetrics = []string{
	"http_rps", "http_2xx_ratio", "http_5xx_ratio", "http_p99_ms",
	"payment_rps", "payment_success_ratio",
	"payment_pending_stuck", "webhook_delivery_ratio",
}

var summaryProviders = []string{"bca", "mandiri", "ewallet"}

// ---------- klien Prometheus ----------

func promInstant(base, query string) ([]promSample, error) {
	client := &http.Client{Timeout: promTimeout}
	u := base + "/api/v1/query?" + url.Values{"query": {query}}.Encode()
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var qr struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]any            `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, err
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("prometheus status %q", qr.Status)
	}
	var out []promSample
	for _, r := range qr.Data.Result {
		tsF, _ := r.Value[0].(float64)
		valS, _ := r.Value[1].(string)
		var v float64
		if _, err := fmt.Sscan(valS, &v); err != nil {
			continue // tak terparse (Stale marker Prometheus) = lewati
		}
		out = append(out, promSample{metric: r.Metric, value: v, ts: time.Unix(int64(tsF), 0).UTC()})
	}
	return out, nil
}

type promPoint struct {
	ts  time.Time
	val *float64
}

func promRange(base, query string, start, end time.Time, step string) ([]promPoint, error) {
	client := &http.Client{Timeout: promRangeTimout}
	q := url.Values{
		"query": {query},
		"start": {start.UTC().Format(time.RFC3339)},
		"end":   {end.UTC().Format(time.RFC3339)},
		"step":  {step},
	}
	resp, err := client.Get(base + "/api/v1/query_range?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var qr struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][2]any `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, err
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("prometheus status %q", qr.Status)
	}
	var out []promPoint
	for _, r := range qr.Data.Result {
		for _, pair := range r.Values {
			tsF, _ := pair[0].(float64)
			valS, _ := pair[1].(string)
			var v float64
			if _, err := fmt.Sscan(valS, &v); err != nil {
				continue
			}
			pt := promPoint{ts: time.Unix(int64(tsF), 0).UTC()}
			// NaN/Inf (mis. 0/0 pada rasio) = gap eksplisit (null),
			// bukan titik. Tanpa ini json.Marshal gagal total.
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				vv := v
				pt.val = &vv
			}
			out = append(out, pt)
		}
	}
	return out, nil
}

// ---------- semantik ----------

func f64ptr(v float64) *float64 { return &v }
func strptr(s string) *string   { return &s }

func classifyValue(samples []promSample, now time.Time) metricValue {
	if len(samples) == 0 {
		return metricValue{Status: "no_data", Detail: "vektor kosong (bukan 0)"}
	}
	s := samples[0]
	if math.IsNaN(s.value) || math.IsInf(s.value, 0) {
		return metricValue{Status: "no_data", LastDataAt: strptr(rfc3339(s.ts)), Detail: "hasil NaN/Inf (0/0?)"}
	}
	mv := metricValue{Status: "ok", Value: f64ptr(s.value), LastDataAt: strptr(rfc3339(s.ts))}
	if now.Sub(s.ts) > staleAfter {
		mv.Status = "stale"
		mv.Value = nil
		mv.Detail = "titik terakhir lebih tua dari 15 dtk"
	}
	return mv
}

func sourceDown(detail string) metricValue {
	return metricValue{Status: "source_down", Detail: detail}
}
