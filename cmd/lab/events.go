package main

// SSE /api/v1/events (U1): event `health` + `metrics` tiap 5 dtk.
// EventSource reconnect otomatis; `retry: 3000`. Klien fallback ke polling
// bila SSE gagal (lihat ui useEvents).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	demo, err := demoScenario(r, s.cfg.demo)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Param buruk", err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "SSE tak didukung", "ResponseWriter tanpa Flush")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	sendMetrics := func() bool {
		if demo != "" {
			sum := demoSummary(demo, "5m")
			return writeSSE(w, flusher, "metrics", mustJSON(sum))
		}
		now := time.Now()
		base := promBaseURL()
		mm := map[string]any{}
		for _, name := range []string{"http_2xx_ratio", "payment_success_ratio", "payment_rps"} {
			mm[name] = queryMetric(base, metricQueries[name], "5m", "", now)
		}
		return writeSSE(w, flusher, "metrics", mustJSON(map[string]any{
			"checkedAt": rfc3339(now), "window": "5m", "metrics": mm,
		}))
	}
	sendHealth := func() bool {
		if demo != "" {
			return writeSSE(w, flusher, "health", mustJSON(map[string]any{"labUp": true}))
		}
		st := probeStack()
		return writeSSE(w, flusher, "health", mustJSON(map[string]any{"labUp": st.labUp}))
	}

	// U4: keadaan insiden terbuka + approval pending untuk event realtime.
	// Kueri dibungkus timeout konteks request; gagal = lewati tick ini
	// (degradasi anggun, bukan putus SSE).
	incidentState := func() (string, string, bool) {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		items, err := s.incidents.List(ctx, "open", "", "")
		if err != nil {
			return "", "", false
		}
		type mini struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Severity string `json:"severity"`
		}
		minis := make([]mini, 0, len(items))
		ids := make([]string, 0, len(items))
		for _, in := range items {
			minis = append(minis, mini{ID: in.ID, Title: in.Title, Severity: in.Severity})
			ids = append(ids, in.ID+"="+in.Status)
		}
		return strings.Join(ids, ","), mustJSON(map[string]any{"open": minis}), true
	}
	approvalState := func() (string, string, bool) {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		items, err := s.incidents.ListApprovals(ctx, "", "pending")
		if err != nil {
			return "", "", false
		}
		type mini struct {
			ID         string `json:"id"`
			IncidentID string `json:"incidentId"`
			Action     string `json:"action"`
		}
		minis := make([]mini, 0, len(items))
		ids := make([]string, 0, len(items))
		for _, a := range items {
			minis = append(minis, mini{ID: a.ID, IncidentID: a.IncidentID, Action: a.Action})
			ids = append(ids, a.ID)
		}
		return strings.Join(ids, ","), mustJSON(map[string]any{"pending": minis}), true
	}

	// Ledakan awal agar first paint cepat (penting untuk test + Playwright).
	if !sendHealth() || !sendMetrics() {
		return
	}
	lastIncidentSig, lastApprovalSig := "", ""
	if sig, data, ok := incidentState(); ok {
		lastIncidentSig = sig
		if !writeSSE(w, flusher, "incident", data) {
			return
		}
	}
	if sig, data, ok := approvalState(); ok {
		lastApprovalSig = sig
		if !writeSSE(w, flusher, "approval", data) {
			return
		}
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !sendHealth() || !sendMetrics() {
				return
			}
			// Incident/approval: kirim hanya saat berubah (toast hemat).
			if sig, data, ok := incidentState(); ok && sig != lastIncidentSig {
				lastIncidentSig = sig
				if !writeSSE(w, flusher, "incident", data) {
					return
				}
			}
			if sig, data, ok := approvalState(); ok && sig != lastApprovalSig {
				lastApprovalSig = sig
				if !writeSSE(w, flusher, "approval", data) {
					return
				}
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event, data string) bool {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(singleLine(b))
}

func singleLine(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c != '\n' {
			out = append(out, c)
		}
	}
	return out
}
