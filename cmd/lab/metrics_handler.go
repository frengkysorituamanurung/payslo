package main

// Handler U1: /metrics/summary, /metrics/series, /events (SSE).
// Param `demo` hanya berlaku bila server --demo; mode live + param = 400.

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func promBaseURL() string {
	if v := os.Getenv("PROM_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "http://127.0.0.1:9090"
}

// demoScenario: nama skenario fixture. Mode --demo tanpa param = "healthy".
// Mode live + param demo = 400.
func demoScenario(r *http.Request, demoMode bool) (string, error) {
	d := r.URL.Query().Get("demo")
	if d == "" {
		if demoMode {
			return "healthy", nil
		}
		return "", nil
	}
	if !demoMode {
		return "", fmt.Errorf("param demo hanya berlaku pada mode --demo")
	}
	if !validDemos[d] {
		return "", fmt.Errorf("demo %q tidak dikenal (healthy|bca_down|prom_down|empty|stale)", d)
	}
	return d, nil
}

func (s *server) handleSummary(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "5m"
	}
	if !validWindows[window] {
		writeProblem(w, http.StatusBadRequest, "Window buruk", "window harus 1m|5m|15m|1h")
		return
	}
	demo, err := demoScenario(r, s.cfg.demo)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Param buruk", err.Error())
		return
	}
	if demo != "" {
		writeJSON(w, http.StatusOK, demoSummary(demo, window))
		return
	}
	now := time.Now()
	out := map[string]any{
		"checkedAt": rfc3339(now),
		"window":    window,
		"metrics":   map[string]any{},
	}
	mm := out["metrics"].(map[string]any)
	base := promBaseURL()
	promDown := false
	for _, name := range summaryMetrics {
		q := metricQueries[name]
		mv := queryMetric(base, q, window, "", now)
		if mv.Status == "source_down" {
			promDown = true
		}
		mm[name] = mv
	}
	for _, p := range summaryProviders {
		q := metricQueries["payment_success_ratio_by_provider"]
		mv := queryMetric(base, q, window, p, now)
		if mv.Status == "source_down" {
			promDown = true
		}
		mm["payment_success_ratio_by_provider:"+p] = mv
	}
	_ = promDown
	writeJSON(w, http.StatusOK, out)
}

func queryMetric(base string, q namedQuery, window, provider string, now time.Time) metricValue {
	samples, err := promInstant(base, q.build(window, provider))
	if err != nil {
		return sourceDown("prometheus tak terjangkau: " + shortErr(err))
	}
	mv := classifyValue(samples, now)
	if q.rate != nil && mv.Status == "ok" {
		if rs, err := promInstant(base, q.rate(window, provider)); err == nil && len(rs) > 0 {
			v := rs[0].value
			mv.SampleRate = &v
		}
	}
	return mv
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 120 {
		return s[:120]
	}
	return s
}

func (s *server) handleSeries(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	q, ok := metricQueries[name]
	if !ok {
		writeProblem(w, http.StatusBadRequest, "Metric tak dikenal", "name harus salah satu allowlist (lihat openapi)")
		return
	}
	provider := r.URL.Query().Get("provider")
	if q.needsProvider {
		if !validProviders[provider] {
			writeProblem(w, http.StatusBadRequest, "Provider buruk", "provider wajib bca|mandiri|ewallet untuk metric ini")
			return
		}
	} else if provider != "" {
		writeProblem(w, http.StatusBadRequest, "Provider tak dipakai", "metric ini tidak memakai provider")
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "15m"
	}
	if !validWindows[window] {
		writeProblem(w, http.StatusBadRequest, "Window buruk", "window harus 1m|5m|15m|1h")
		return
	}
	step := r.URL.Query().Get("step")
	if step == "" {
		step = "15s"
	}
	if !validSteps[step] {
		writeProblem(w, http.StatusBadRequest, "Step buruk", "step harus 5s|15s|1m")
		return
	}
	if windowSeconds(window)/stepSeconds(step) > maxSeriesPoints {
		writeProblem(w, http.StatusBadRequest, "Terlalu banyak titik", "kecilkan window atau besarkan step (maks 1000 titik)")
		return
	}
	demo, err := demoScenario(r, s.cfg.demo)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Param buruk", err.Error())
		return
	}
	if demo != "" {
		writeJSON(w, http.StatusOK, demoSeries(demo, name, step))
		return
	}
	now := time.Now()
	start := now.Add(-time.Duration(windowSeconds(window)) * time.Second)
	points, err := promRange(promBaseURL(), q.build(window, provider), start, now, step)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"name": name, "step": step, "status": "source_down",
			"lastDataAt": nil, "points": []any{},
		})
		return
	}
	pts := make([]any, 0, len(points))
	var last *string
	for _, p := range points {
		var v any
		if p.val != nil {
			v = *p.val
		}
		pts = append(pts, []any{rfc3339(p.ts), v})
		last = strptr(rfc3339(p.ts))
	}
	status := "ok"
	if len(pts) == 0 {
		status = "no_data"
	} else if last != nil {
		if lt, err := time.Parse(time.RFC3339, *last); err == nil && now.Sub(lt) > staleAfter {
			status = "stale"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name, "step": step, "status": status,
		"lastDataAt": last, "points": pts,
	})
}
