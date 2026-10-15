package main

// Fixture --demo U1 (deterministik: waktu dasar tetap agar snapshot stabil).
// Skenario: healthy | bca_down | prom_down | empty | stale.

import (
	"math"
	"time"
)

const demoBase = "2026-10-16T07:00:00Z"

func demoTime() time.Time {
	t, _ := time.Parse(time.RFC3339, demoBase)
	return t
}

func demoMV(status string, value, rate float64, ago time.Duration) metricValue {
	mv := metricValue{Status: status}
	if status == "ok" {
		mv.Value = f64ptr(value)
		mv.SampleRate = f64ptr(rate)
		mv.LastDataAt = strptr(rfc3339(demoTime()))
		return mv
	}
	if status == "stale" {
		mv.LastDataAt = strptr(rfc3339(demoTime().Add(-ago)))
		mv.Detail = "titik terakhir lebih tua dari 15 dtk"
		return mv
	}
	if status == "no_data" {
		mv.Detail = "vektor kosong (bukan 0)"
		return mv
	}
	mv.Detail = "prometheus tak terjangkau (fixture)"
	return mv
}

func demoSummary(scenario, window string) map[string]any {
	mm := map[string]any{}
	switch scenario {
	case "prom_down":
		for _, n := range append(append([]string{}, summaryMetrics...), "payment_success_ratio_by_provider:bca", "payment_success_ratio_by_provider:mandiri", "payment_success_ratio_by_provider:ewallet") {
			mm[n] = demoMV("source_down", 0, 0, 0)
		}
	case "empty":
		for _, n := range append(append([]string{}, summaryMetrics...), "payment_success_ratio_by_provider:bca", "payment_success_ratio_by_provider:mandiri", "payment_success_ratio_by_provider:ewallet") {
			mm[n] = demoMV("no_data", 0, 0, 0)
		}
	case "stale":
		for _, n := range append(append([]string{}, summaryMetrics...), "payment_success_ratio_by_provider:bca", "payment_success_ratio_by_provider:mandiri", "payment_success_ratio_by_provider:ewallet") {
			mm[n] = demoMV("stale", 0, 0, 60*time.Second)
		}
	case "bca_down":
		mm["http_rps"] = demoMV("ok", 2.1, 2.1, 0)
		mm["http_2xx_ratio"] = demoMV("ok", 0.998, 2.1, 0)
		mm["http_5xx_ratio"] = demoMV("ok", 0.001, 2.1, 0)
		mm["http_p99_ms"] = demoMV("ok", 210, 2.1, 0)
		mm["payment_rps"] = demoMV("ok", 1.9, 1.9, 0)
		mm["payment_success_ratio"] = demoMV("ok", 0.62, 1.9, 0)
		mm["payment_success_ratio_by_provider:bca"] = demoMV("ok", 0.05, 0.8, 0)
		mm["payment_success_ratio_by_provider:mandiri"] = demoMV("ok", 0.99, 0.6, 0)
		mm["payment_success_ratio_by_provider:ewallet"] = demoMV("ok", 0.995, 0.5, 0)
		mm["payment_pending_stuck"] = demoMV("ok", 12, 0, 0)
		mm["webhook_delivery_ratio"] = demoMV("ok", 1.0, 1.9, 0)
	default: // healthy
		mm["http_rps"] = demoMV("ok", 2.1, 2.1, 0)
		mm["http_2xx_ratio"] = demoMV("ok", 0.999, 2.1, 0)
		mm["http_5xx_ratio"] = demoMV("ok", 0.0, 2.1, 0)
		mm["http_p99_ms"] = demoMV("ok", 180, 2.1, 0)
		mm["payment_rps"] = demoMV("ok", 1.9, 1.9, 0)
		mm["payment_success_ratio"] = demoMV("ok", 0.985, 1.9, 0)
		mm["payment_success_ratio_by_provider:bca"] = demoMV("ok", 0.99, 0.8, 0)
		mm["payment_success_ratio_by_provider:mandiri"] = demoMV("ok", 0.98, 0.6, 0)
		mm["payment_success_ratio_by_provider:ewallet"] = demoMV("ok", 0.995, 0.5, 0)
		mm["payment_pending_stuck"] = demoMV("ok", 0, 0, 0)
		mm["webhook_delivery_ratio"] = demoMV("ok", 1.0, 1.9, 0)
	}
	return map[string]any{"checkedAt": demoBase, "window": window, "metrics": mm}
}

func demoSeries(scenario, name, step string) map[string]any {
	base := demoTime()
	n := 60
	end := base
	status := "ok"
	switch scenario {
	case "prom_down":
		return map[string]any{"name": name, "step": step, "status": "source_down", "lastDataAt": nil, "points": []any{}}
	case "empty":
		return map[string]any{"name": name, "step": step, "status": "no_data", "lastDataAt": nil, "points": []any{}}
	case "stale":
		end = base.Add(-60 * time.Second)
		status = "stale"
	}
	pts := make([]any, 0, n)
	stepDur := time.Duration(stepSeconds(step)) * time.Second
	for i := n - 1; i >= 0; i-- {
		ts := end.Add(-time.Duration(i) * stepDur)
		v := demoPointValue(scenario, name, n-1-i)
		pts = append(pts, []any{rfc3339(ts), v})
	}
	var last any
	if len(pts) > 0 {
		if pair, ok := pts[len(pts)-1].([]any); ok {
			last = pair[0]
		}
	}
	return map[string]any{"name": name, "step": step, "status": status, "lastDataAt": last, "points": pts}
}

func demoPointValue(scenario, name string, idx int) float64 {
	jitter := 0.002 * math.Sin(float64(idx)*0.7)
	base := 0.99
	switch name {
	case "http_2xx_ratio":
		base = 0.999
	case "http_5xx_ratio":
		return 0.0 + jitter/10
	case "http_p99_ms":
		return 180 + 20*math.Sin(float64(idx)*0.3)
	case "payment_success_ratio":
		base = 0.985
	}
	if scenario == "bca_down" && (name == "payment_success_ratio" || name == "payment_success_ratio_by_provider") {
		if idx >= 30 {
			return 0.55 + jitter
		}
	}
	return base + jitter
}
