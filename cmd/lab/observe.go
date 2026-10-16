package main

// Observability + diagnostik U7: status koneksi sumber, info embed Grafana,
// anotasi (filter sesuai mode), alert/rules Prometheus, alert Alertmanager,
// riwayat ntfy, laporan diagnostik BEBAS SECRET, dan ambang data-sedikit di
// pengaturan (server-side, dipakai UI sebagai default).
//
// Prinsip: setiap sumber yang mati menghasilkan status jujur (reachable=false
// / daftar kosong + 502 yang ditangani UI), BUKAN error tak tertangani.
// Laporan diagnostik tidak pernah memuat nilai env, token, atau password —
// hanya nama service, versi (ref image / buildinfo publik), dan latensi.
// Dipindai otomatis di u7_test.go (TestDiagnosticsSecretFree).

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ------------------------------------------------------------------ endpoint URLs

func alertmanagerBaseURL() string {
	if v := os.Getenv("LAB_ALERTMANAGER_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "http://127.0.0.1:9093"
}

func ntfyBaseURL() string {
	if v := os.Getenv("LAB_NTFY_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "http://127.0.0.1:8022"
}

func ntfyTopic() string {
	if v := os.Getenv("LAB_NTFY_TOPIC"); v != "" {
		return v
	}
	return "payslo-lab"
}

func grafanaPassword() string {
	if v := os.Getenv("GRAFANA_PASSWORD"); v != "" {
		return v
	}
	return "lab-only-grafana"
}

// ------------------------------------------------------------------ probe generik

type probedService struct {
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
	LatencyMs int    `json:"latencyMs"`
	Detail    string `json:"detail,omitempty"`
}

func probeHTTP(ctx context.Context, name, url string) probedService {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return probedService{Name: name, LatencyMs: int(time.Since(start).Milliseconds()), Detail: "request buruk"}
	}
	resp, err := http.DefaultClient.Do(req)
	lat := int(time.Since(start).Milliseconds())
	if err != nil {
		return probedService{Name: name, LatencyMs: lat, Detail: "tak terjangkau"}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode >= 500 {
		return probedService{Name: name, LatencyMs: lat, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return probedService{Name: name, Reachable: true, LatencyMs: lat}
}

func getJSON(ctx context.Context, url string, headers map[string]string, limit int64) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// ------------------------------------------------------------------ docker helpers

// composeServices: nama service compose yang berjalan (untuk deteksi profil
// obs-full: loki/tempo/alloy). Gagal (Docker mati) -> nil + available=false.
// Timeout keras agar UI tidak menggantung.
func composeServices() (names map[string]string, available bool) {
	names = map[string]string{}
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	var out bytes.Buffer
	cmd.Stdout = &out
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			return nil, false
		}
	case <-time.After(6 * time.Second):
		_ = cmd.Process.Kill()
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	for {
		var e struct {
			Service string `json:"Service"`
			Image   string `json:"Image"`
			State   string `json:"State"`
		}
		if err := dec.Decode(&e); err != nil {
			break
		}
		if e.Service == "" || e.State != "running" {
			continue
		}
		names[e.Service] = e.Image
	}
	return names, true
}

type dockerStat struct {
	Name    string `json:"name"`
	CPUPerc string `json:"cpuPerc"`
	MemUse  string `json:"memUsage"`
	MemPerc string `json:"memPerc"`
}

// dockerStats: `docker stats --no-stream` (JSON per baris). Gagal -> available=false
// dengan pesan jujur "Docker tidak tersedia" (bukan error).
func dockerStats() (stats []dockerStat, available bool, note string) {
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", "{{json .}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			return nil, false, "Docker tidak tersedia (jalankan Docker Desktop / dockerd)"
		}
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		return nil, false, "Docker tidak tersedia (waktu habis membaca stats)"
	}
	stats = []dockerStat{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw struct {
			Name     string `json:"Name"`
			CPUPerc  string `json:"CPUPerc"`
			MemUsage string `json:"MemUsage"`
			MemPerc  string `json:"MemPerc"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil || raw.Name == "" {
			continue
		}
		stats = append(stats, dockerStat{Name: raw.Name, CPUPerc: raw.CPUPerc, MemUse: raw.MemUsage, MemPerc: raw.MemPerc})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Name < stats[j].Name })
	return stats, true, ""
}

// ------------------------------------------------------------------ GET /api/v1/observe/status

func (s *server) handleObserveStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, demoObserveStatus())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	services := []probedService{
		probeHTTP(ctx, "grafana", s.cfg.grafana+"/api/health"),
		probeHTTP(ctx, "prometheus", promBaseURL()+"/-/healthy"),
		probeHTTP(ctx, "alertmanager", alertmanagerBaseURL()+"/-/healthy"),
		probeHTTP(ctx, "ntfy", ntfyBaseURL()+"/v1/health"),
		probeHTTP(ctx, "loki", lokiBaseURL()+"/ready"),
		probeHTTP(ctx, "tempo", tempoBaseURL()+"/ready"),
	}
	running, dockerOK := composeServices()
	profiles := map[string]bool{"obs-full": false}
	dockerNote := ""
	if !dockerOK {
		dockerNote = "Docker tidak tersedia (jalankan Docker Desktop / dockerd)"
	} else {
		_, hasLoki := running["loki"]
		_, hasTempo := running["tempo"]
		_, hasAlloy := running["alloy"]
		profiles["obs-full"] = hasLoki && hasTempo && hasAlloy
		if !profiles["obs-full"] {
			dockerNote = "profil obs-full tidak aktif (loki/tempo/alloy mati): fitur log & trace dinonaktifkan"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"services": services, "dockerAvailable": dockerOK, "dockerNote": dockerNote,
		"composeProfiles": profiles, "checkedAt": rfc3339(time.Now()),
	})
}

func lokiBaseURL() string {
	if v := os.Getenv("LAB_LOKI_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "http://127.0.0.1:3100"
}

func tempoBaseURL() string {
	if v := os.Getenv("LAB_TEMPO_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "http://127.0.0.1:3200"
}

func demoObserveStatus() map[string]any {
	mk := func(name string, reachable bool, detail string) map[string]any {
		return map[string]any{"name": name, "reachable": reachable, "latencyMs": 3, "detail": detail}
	}
	return map[string]any{
		"services": []any{
			mk("grafana", true, ""), mk("prometheus", true, ""),
			mk("alertmanager", true, ""), mk("ntfy", true, ""),
			mk("loki", false, "profil obs-full tidak aktif (demo)"),
			mk("tempo", false, "profil obs-full tidak aktif (demo)"),
		},
		"dockerAvailable": false,
		"dockerNote":      "Docker tidak tersedia (mode demo)",
		"composeProfiles": map[string]bool{"obs-full": false},
		"checkedAt":       "2026-10-16T07:00:00Z",
	}
}

// ------------------------------------------------------------------ GET /api/v1/observe/grafana

type grafanaDashboard struct {
	Title string `json:"title"`
	UID   string `json:"uid"`
}

func (s *server) handleGrafanaEmbed(w http.ResponseWriter, r *http.Request) {
	dashboards := []grafanaDashboard{
		{Title: "Payment Overview", UID: "payment-overview"},
		{Title: "Provider Health", UID: "provider-health"},
		{Title: "SLO Overview", UID: "slo-overview"},
		{Title: "Incident Triage", UID: "incident-triage"},
	}
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{
			"url": s.cfg.grafana, "kioskUrl": s.cfg.grafana + "/d/payment-overview?orgId=1&kiosk",
			"dashboards": dashboards, "embeddingAllowed": true,
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	reachable := probeHTTP(ctx, "grafana", s.cfg.grafana+"/api/health").Reachable
	writeJSON(w, http.StatusOK, map[string]any{
		"url": s.cfg.grafana, "kioskUrl": s.cfg.grafana + "/d/payment-overview?orgId=1&kiosk",
		"dashboards": dashboards, "embeddingAllowed": reachable,
	})
}

// ------------------------------------------------------------------ GET /api/v1/observe/annotations

func (s *server) handleAnnotations(w http.ResponseWriter, r *http.Request) {
	mentor := s.mentor.check(r.Header.Get("X-Mentor-Token"))
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{"annotations": demoAnnotations(mentor)})
		return
	}
	from := r.URL.Query().Get("from")
	if from == "" {
		from = strconv.FormatInt(time.Now().Add(-time.Hour).Unix()*1000, 10)
	}
	to := r.URL.Query().Get("to")
	if to == "" {
		to = strconv.FormatInt(time.Now().Unix()*1000, 10)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	code, body, err := getJSON(ctx,
		s.cfg.grafana+"/api/annotations?from="+from+"&to="+to,
		map[string]string{"Authorization": "Basic " + basicAuth("admin", grafanaPassword())}, 1<<20)
	if err != nil || code != http.StatusOK {
		writeProblem(w, http.StatusBadGateway, "Grafana tak terjangkau", "anotasi tidak dapat dibaca")
		return
	}
	var raw []struct {
		ID    int64    `json:"id"`
		Time  int64    `json:"time"`
		Text  string   `json:"text"`
		Tags  []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		writeProblem(w, http.StatusBadGateway, "Grafana tak terbaca", "respons anotasi bukan JSON")
		return
	}
	out := []map[string]any{}
	for _, a := range raw {
		if !mentor && hasChaosTag(a.Tags) {
			continue // trainee tidak melihat anotasi chaos (anti-spoiler)
		}
		tags := a.Tags
		if tags == nil {
			tags = []string{}
		}
		out = append(out, map[string]any{
			"id": a.ID, "time": rfc3339(time.UnixMilli(a.Time)), "text": a.Text, "tags": tags,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"annotations": out})
}

func hasChaosTag(tags []string) bool {
	for _, t := range tags {
		if strings.EqualFold(strings.TrimSpace(t), "chaos") {
			return true
		}
	}
	return false
}

func demoAnnotations(mentor bool) []map[string]any {
	all := []map[string]any{
		{"id": 1, "time": "2026-10-16T06:50:00Z", "text": "gateway deploy good via lab", "tags": []string{"deploy", "gateway"}},
		{"id": 2, "time": "2026-10-16T06:55:00Z", "text": "chaos: bank bca -> down", "tags": []string{"chaos"}},
		{"id": 3, "time": "2026-10-16T06:58:00Z", "text": "traffic rate=6/s", "tags": []string{"traffic"}},
	}
	if mentor {
		return all
	}
	out := []map[string]any{}
	for _, a := range all {
		tags, _ := a["tags"].([]string)
		if hasChaosTag(tags) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ------------------------------------------------------------------ Prometheus / Alertmanager / ntfy

func (s *server) handlePrometheusAlerts(w http.ResponseWriter, r *http.Request) {
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{"alerts": []any{
			map[string]any{"labels": map[string]string{"alertname": "PaySLOBurnRateHigh", "slo": "payment-success-rate", "severity": "page"}, "state": "firing", "activeAt": "2026-10-16T07:00:00Z"},
		}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	code, body, err := getJSON(ctx, promBaseURL()+"/api/v1/alerts", nil, 1<<20)
	if err != nil || code != http.StatusOK {
		writeProblem(w, http.StatusBadGateway, "Prometheus tak terjangkau", "alert tidak dapat dibaca")
		return
	}
	var decoded struct {
		Data struct {
			Alerts []struct {
				Labels   map[string]string `json:"labels"`
				State    string            `json:"state"`
				ActiveAt string            `json:"activeAt"`
			} `json:"alerts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		writeProblem(w, http.StatusBadGateway, "Prometheus tak terbaca", "respons bukan JSON")
		return
	}
	out := []map[string]any{}
	for _, a := range decoded.Data.Alerts {
		out = append(out, map[string]any{"labels": a.Labels, "state": a.State, "activeAt": a.ActiveAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out})
}

func (s *server) handlePrometheusRules(w http.ResponseWriter, r *http.Request) {
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{"groups": []any{
			map[string]any{"name": "payslo-payment-success-rate", "rules": []string{"PaySLOBurnRateHigh", "payslo:slo_error_ratio:payment_success_rate:5m"}},
		}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	code, body, err := getJSON(ctx, promBaseURL()+"/api/v1/rules", nil, 1<<20)
	if err != nil || code != http.StatusOK {
		writeProblem(w, http.StatusBadGateway, "Prometheus tak terjangkau", "rules tidak dapat dibaca")
		return
	}
	var decoded struct {
		Data struct {
			Groups []struct {
				Name  string `json:"name"`
				Rules []struct {
					Name string `json:"name"`
				} `json:"rules"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		writeProblem(w, http.StatusBadGateway, "Prometheus tak terbaca", "respons bukan JSON")
		return
	}
	out := []map[string]any{}
	for _, g := range decoded.Data.Groups {
		if !strings.HasPrefix(g.Name, "payslo") {
			continue // hanya grup milik payslo
		}
		names := []string{}
		for _, rule := range g.Rules {
			names = append(names, rule.Name)
		}
		out = append(out, map[string]any{"name": g.Name, "rules": names})
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

func (s *server) handleAlertmanager(w http.ResponseWriter, r *http.Request) {
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{"alerts": []any{
			map[string]any{"labels": map[string]string{"alertname": "PaySLOBurnRateHigh", "severity": "page"}, "state": "active"},
		}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	code, body, err := getJSON(ctx, alertmanagerBaseURL()+"/api/v2/alerts", nil, 1<<20)
	if err != nil || code != http.StatusOK {
		writeProblem(w, http.StatusBadGateway, "Alertmanager tak terjangkau", "alert tidak dapat dibaca")
		return
	}
	var raw []struct {
		Labels map[string]string `json:"labels"`
		Status struct {
			State string `json:"state"`
		} `json:"status"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		writeProblem(w, http.StatusBadGateway, "Alertmanager tak terbaca", "respons bukan JSON")
		return
	}
	out := []map[string]any{}
	for _, a := range raw {
		out = append(out, map[string]any{"labels": a.Labels, "state": a.Status.State})
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": out})
}

func (s *server) handleNtfyHistory(w http.ResponseWriter, r *http.Request) {
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{"source": "mock", "messages": []any{
			map[string]any{"id": "m1", "time": "2026-10-16T07:00:00Z", "topic": "payslo-lab", "message": "incident opened: payment-success-rate on gateway-api (page)"},
			map[string]any{"id": "m2", "time": "2026-10-16T07:05:00Z", "topic": "payslo-lab", "message": "approval approved by sinta: disable provider bca"},
		}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	code, body, err := getJSON(ctx, ntfyBaseURL()+"/"+ntfyTopic()+"/json?poll=1",
		map[string]string{"Accept": "application/json"}, 1<<20)
	if err != nil || (code != http.StatusOK && code != http.StatusNoContent) {
		writeProblem(w, http.StatusBadGateway, "ntfy tak terjangkau", "riwayat tidak dapat dibaca")
		return
	}
	out := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m struct {
			ID      string `json:"id"`
			Time    int64  `json:"time"`
			Topic   string `json:"topic"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &m); err != nil || m.ID == "" {
			continue
		}
		out = append(out, map[string]any{
			"id": m.ID, "time": rfc3339(time.Unix(m.Time, 0)), "topic": m.Topic, "message": m.Message,
		})
		if len(out) >= 50 {
			break
		}
	}
	// Terbaru dulu.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": "ntfy", "messages": out})
}

// ------------------------------------------------------------------ GET /api/v1/diagnostics

// diagnosticsReport: BEBAS SECRET. Hanya nama service, ref image/versi publik,
// status koneksi, latensi, dan NAMA key konfigurasi (tanpa nilai).
func (s *server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	type svcDiag struct {
		Name      string            `json:"name"`
		Version   string            `json:"version"`
		Reachable bool              `json:"reachable"`
		Config    map[string]string `json:"config"`
	}
	if s.cfg.demo {
		mk := func(name, version string, reachable bool) svcDiag {
			return svcDiag{Name: name, Version: version, Reachable: reachable, Config: map[string]string{}}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"services": []svcDiag{
				mk("labserve", "dev-demo", true),
				mk("grafana", "unknown", true),
				mk("prometheus", "unknown", true),
				mk("postgres", "unknown", false),
			},
			"docker":      map[string]any{"available": false, "note": "Docker tidak tersedia (mode demo)"},
			"checkedAt":   "2026-10-16T07:00:00Z",
			"generatedBy": "labserve",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	running, dockerOK := composeServices()
	stats, statsOK, statsNote := dockerStats()
	// Versi publik dari buildinfo (tanpa auth, tanpa secret).
	promVersion := promBuildVersion(ctx)
	grafanaVersion := s.grafanaBuildVersion(ctx)
	versionOf := func(service, fallback string) string {
		if img, ok := running[service]; ok && img != "" {
			return img // ref image compose, mis. grafana/grafana:11.x (tanpa kredensial)
		}
		return fallback
	}
	probes := map[string]probedService{}
	for _, p := range []probedService{
		probeHTTP(ctx, "postgres", s.cfg.gateway+"/healthz"),
		probeHTTP(ctx, "grafana", s.cfg.grafana+"/api/health"),
		probeHTTP(ctx, "prometheus", promBaseURL()+"/-/healthy"),
		probeHTTP(ctx, "alertmanager", alertmanagerBaseURL()+"/-/healthy"),
		probeHTTP(ctx, "ntfy", ntfyBaseURL()+"/v1/health"),
		probeHTTP(ctx, "loki", lokiBaseURL()+"/ready"),
		probeHTTP(ctx, "tempo", tempoBaseURL()+"/ready"),
	} {
		probes[p.Name] = p
	}
	services := []svcDiag{}
	for _, name := range []string{"postgres", "grafana", "prometheus", "alertmanager", "ntfy", "loki", "tempo"} {
		version := versionOf(name, "tidak dilaporkan")
		if name == "prometheus" && promVersion != "" {
			version = promVersion
		}
		if name == "grafana" && grafanaVersion != "" {
			version = grafanaVersion
		}
		p := probes[name]
		services = append(services, svcDiag{Name: name, Version: version, Reachable: p.Reachable, Config: map[string]string{}})
	}
	docker := map[string]any{"available": dockerOK && statsOK}
	if dockerOK && statsOK {
		docker["stats"] = stats
	} else {
		docker["note"] = statsNote
		if dockerNote := statsNote; dockerNote == "" {
			docker["note"] = "Docker tidak tersedia (jalankan Docker Desktop / dockerd)"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"services": services, "docker": docker,
		"checkedAt": rfc3339(time.Now()), "generatedBy": "labserve",
	})
}

func promBuildVersion(ctx context.Context) string {
	code, body, err := getJSON(ctx, promBaseURL()+"/api/v1/status/buildinfo", nil, 1<<16)
	if err != nil || code != http.StatusOK {
		return ""
	}
	var decoded struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ""
	}
	return decoded.Data.Version
}

func (s *server) grafanaBuildVersion(ctx context.Context) string {
	code, body, err := getJSON(ctx, s.cfg.grafana+"/api/health", nil, 1<<16)
	if err != nil || code != http.StatusOK {
		return ""
	}
	var decoded struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return ""
	}
	return decoded.Version
}

// basicAuth: untuk Grafana loopback; kredensial TIDAK PERNAH masuk respons.
func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}
