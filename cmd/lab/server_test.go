package main

// Test keamanan (UI-SPEC §KEAMANAN) + kontrak vs api/openapi.yaml (U0).

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const testPort = "8088"
const testToken = "test-token-123"

func testServer() *server {
	return newServer(serverConfig{demo: true, webDir: "testdata/web", token: testToken, port: testPort})
}

func doReq(t *testing.T, h http.Handler, method, target, host string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func goodHeaders() map[string]string { return map[string]string{} }

// ---------- keamanan ----------

func TestBadHostRejected(t *testing.T) {
	s := testServer()
	for _, host := range []string{"evil.com", "127.0.0.1:9999", "localhost.evil.com", "127.0.0.1.evil.com:8088"} {
		rec := doReq(t, s.handler(), "GET", "/api/v1/session", host, goodHeaders())
		if rec.Code != http.StatusForbidden {
			t.Fatalf("host %q: got %d, want 403", host, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("host %q: content-type %q, want problem+json", host, ct)
		}
	}
}

func TestGoodHostsAllowed(t *testing.T) {
	s := testServer()
	for _, host := range []string{"127.0.0.1:8088", "localhost:8088", "[::1]:8088"} {
		rec := doReq(t, s.handler(), "GET", "/api/v1/session", host, goodHeaders())
		if rec.Code != http.StatusOK {
			t.Fatalf("host %q: got %d, want 200", host, rec.Code)
		}
	}
}

func TestMutationRequiresOriginAndToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := mutationGuard(inner, testToken, testPort)
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"tanpa apa pun", map[string]string{}, 403},
		{"origin benar tanpa token", map[string]string{"Origin": "http://127.0.0.1:8088"}, 403},
		{"token benar tanpa origin", map[string]string{"X-Lab-Token": testToken}, 403},
		{"origin salah", map[string]string{"Origin": "https://evil.com", "X-Lab-Token": testToken}, 403},
		{"token salah", map[string]string{"Origin": "http://127.0.0.1:8088", "X-Lab-Token": "salah"}, 403},
		{"lengkap benar", map[string]string{"Origin": "http://127.0.0.1:8088", "X-Lab-Token": testToken}, 204},
		{"localhost origin ok", map[string]string{"Origin": "http://localhost:8088", "X-Lab-Token": testToken}, 204},
	}
	for _, c := range cases {
		rec := doReq(t, h, "POST", "/api/v1/x", "127.0.0.1:8088", c.headers)
		if rec.Code != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, rec.Code, c.want)
		}
	}
	// GET tidak butuh token.
	rec := doReq(t, h, "GET", "/api/v1/x", "127.0.0.1:8088", goodHeaders())
	if rec.Code != http.StatusNoContent {
		t.Fatalf("GET tanpa token: got %d, want 204", rec.Code)
	}
}

func TestNoCORSHeaders(t *testing.T) {
	s := testServer()
	for _, target := range []string{"/api/v1/session", "/api/v1/health/stack", "/"} {
		rec := doReq(t, s.handler(), "GET", target, "127.0.0.1:8088", goodHeaders())
		for k := range rec.Header() {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Fatalf("%s: header CORS terlarang %q", target, k)
			}
		}
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	s := testServer()
	rec := doReq(t, s.handler(), "GET", "/api/v1/session", "127.0.0.1:8088", goodHeaders())
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self' 'nonce-",
		"frame-src http://127.0.0.1:3002",
		"object-src 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP %q tidak mengandung %q", csp, want)
		}
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
}

func TestTokenInjectedWithMatchingNonce(t *testing.T) {
	s := testServer()
	rec := doReq(t, s.handler(), "GET", "/", "127.0.0.1:8088", goodHeaders())
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "window.__LAB_TOKEN__") {
		t.Fatal("token tidak disuntikkan ke halaman")
	}
	// Nonce di script harus sama dengan nonce di header CSP respons ini.
	csp := rec.Header().Get("Content-Security-Policy")
	start := strings.Index(csp, "'nonce-")
	if start < 0 {
		t.Fatal("CSP tanpa nonce")
	}
	nonce := csp[start+len("'nonce-"):]
	nonce = nonce[:strings.Index(nonce, "'")]
	if !strings.Contains(body, `nonce="`+nonce+`"`) {
		t.Fatal("nonce script != nonce CSP")
	}
}

// ---------- kontrak vs openapi.yaml ----------

type openapiDoc struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

// loadSpecOps mengembalikan "get /api/v1/x" untuk setiap operasi di spec.
func loadSpecOps(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("baca openapi.yaml: %v", err)
	}
	var doc openapiDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	ops := map[string]bool{}
	for p, methods := range doc.Paths {
		for m := range methods {
			ops[strings.ToLower(m)+" "+p] = true
		}
	}
	if len(ops) == 0 {
		t.Fatal("spec tanpa operasi")
	}
	return ops
}

// TestSpecContractMatchesRoutes: setiap operasi di api/openapi.yaml punya route
// terdaftar, dan sebaliknya (contract-first, UI-SPEC no. 4).
func TestSpecContractMatchesRoutes(t *testing.T) {
	s := testServer()
	have := map[string]bool{}
	for _, r := range s.routes {
		have[strings.ToLower(r)] = true
	}
	for op := range loadSpecOps(t) {
		if !have[op] {
			t.Errorf("spec punya operasi tanpa route: %q", op)
		}
	}
	for op := range have {
		if op == "/" {
			continue // fallback SPA, bukan operasi kontrak
		}
		if _, ok := loadSpecOps(t)[op]; !ok {
			t.Errorf("route tanpa operasi di spec: %q", op)
		}
	}
}

// TestSpecPathsServed: operasi GET tanpa parameter path benar-benar dilayani
// (bukan 404/405). Body/shape diuji di test khusus.
func TestSpecPathsServed(t *testing.T) {
	s := testServer()
	for op := range loadSpecOps(t) {
		method, p, ok := strings.Cut(op, " ")
		if !ok || method != "get" || strings.Contains(p, "{") {
			continue // hanya GET statis; templated diuji khusus
		}
		if p == "/api/v1/events" {
			continue // SSE: streaming, diuji di TestEventsSSE
		}
		target := p
		rec := doReq(t, s.handler(), "GET", target, "127.0.0.1:8088", goodHeaders())
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
			t.Fatalf("GET %s: got %d (route tidak terdaftar?)", target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
			t.Fatalf("GET %s: content-type %q", target, ct)
		}
	}
}

func TestSessionShape(t *testing.T) {
	s := testServer()
	rec := doReq(t, s.handler(), "GET", "/api/v1/session", "127.0.0.1:8088", goodHeaders())
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("session bukan JSON: %v", err)
	}
	for _, k := range []string{"mode", "traineeName", "scenario", "labUp", "checkedAt"} {
		if _, ok := body[k]; !ok {
			t.Fatalf("session tanpa field %q", k)
		}
	}
	if mode, _ := body["mode"].(string); mode != "trainee" && mode != "mentor" {
		t.Fatalf("mode = %q", mode)
	}
	if _, ok := body["labUp"].(bool); !ok {
		t.Fatal("labUp bukan bool")
	}
	if ts, _ := body["checkedAt"].(string); !validRFC3339(ts) {
		t.Fatalf("checkedAt bukan RFC3339: %q", ts)
	}
}

func TestStackShape(t *testing.T) {
	s := testServer()
	rec := doReq(t, s.handler(), "GET", "/api/v1/health/stack", "127.0.0.1:8088", goodHeaders())
	var body struct {
		LabUp     bool   `json:"labUp"`
		CheckedAt string `json:"checkedAt"`
		Services  []struct {
			Name   string  `json:"name"`
			State  string  `json:"state"`
			Detail *string `json:"detail"`
		} `json:"services"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("stack bukan JSON: %v", err)
	}
	if !validRFC3339(body.CheckedAt) {
		t.Fatalf("checkedAt bukan RFC3339: %q", body.CheckedAt)
	}
	allowed := map[string]bool{"up": true, "down": true, "degraded": true, "unknown": true}
	for _, svc := range body.Services {
		if svc.Name == "" {
			t.Fatal("service tanpa nama")
		}
		if !allowed[svc.State] {
			t.Fatalf("service %q state %q di luar enum", svc.Name, svc.State)
		}
	}
}

func validRFC3339(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

func demoServer() *server {
	return newServer(serverConfig{demo: true, webDir: "testdata/web", token: testToken, port: testPort})
}

func TestSummaryShapeDemo(t *testing.T) {
	s := demoServer()
	rec := doReq(t, s.handler(), "GET", "/api/v1/metrics/summary?window=5m&demo=healthy", "127.0.0.1:8088", goodHeaders())
	if rec.Code != http.StatusOK {
		t.Fatalf("summary: got %d", rec.Code)
	}
	var body struct {
		CheckedAt string `json:"checkedAt"`
		Window    string `json:"window"`
		Metrics   map[string]struct {
			Status     string   `json:"status"`
			Value      *float64 `json:"value"`
			SampleRate *float64 `json:"sampleRate"`
			LastDataAt *string  `json:"lastDataAt"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("summary bukan JSON: %v", err)
	}
	for _, want := range []string{"http_2xx_ratio", "payment_success_ratio", "payment_success_ratio_by_provider:bca"} {
		m, ok := body.Metrics[want]
		if !ok {
			t.Fatalf("summary tanpa metric %q", want)
		}
		if m.Status != "ok" || m.Value == nil || m.LastDataAt == nil {
			t.Fatalf("metric %q tak lengkap: %+v", want, m)
		}
	}
}

func TestSummaryDemoScenarios(t *testing.T) {
	s := demoServer()
	cases := []struct {
		demo   string
		metric string
		status string
	}{
		{"bca_down", "payment_success_ratio_by_provider:bca", "ok"},
		{"prom_down", "http_2xx_ratio", "source_down"},
		{"empty", "payment_success_ratio", "no_data"},
		{"stale", "http_2xx_ratio", "stale"},
	}
	for _, c := range cases {
		rec := doReq(t, s.handler(), "GET", "/api/v1/metrics/summary?demo="+c.demo, "127.0.0.1:8088", goodHeaders())
		if rec.Code != http.StatusOK {
			t.Fatalf("demo %s: got %d", c.demo, rec.Code)
		}
		var body struct {
			Metrics map[string]struct {
				Status string `json:"status"`
			} `json:"metrics"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("demo %s bukan JSON: %v", c.demo, err)
		}
		if body.Metrics[c.metric].Status != c.status {
			t.Fatalf("demo %s metric %s: got %q, want %q", c.demo, c.metric, body.Metrics[c.metric].Status, c.status)
		}
	}
	// Param demo tak dikenal -> 400; window buruk -> 400; live+demo -> 400.
	for _, target := range []string{
		"/api/v1/metrics/summary?demo=ngawur",
		"/api/v1/metrics/summary?window=9m",
		"/api/v1/metrics/series?name=nope",
		"/api/v1/metrics/series?name=payment_success_ratio_by_provider",
	} {
		rec := doReq(t, s.handler(), "GET", target, "127.0.0.1:8088", goodHeaders())
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s: got %d, want 400", target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("GET %s: content-type %q", target, ct)
		}
	}
	live := newServer(serverConfig{demo: false, webDir: "testdata/web", token: testToken, port: testPort})
	rec := doReq(t, live.handler(), "GET", "/api/v1/metrics/summary?demo=healthy", "127.0.0.1:8088", goodHeaders())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("live+demo: got %d, want 400", rec.Code)
	}
}

func TestSeriesShapeDemo(t *testing.T) {
	s := demoServer()
	rec := doReq(t, s.handler(), "GET", "/api/v1/metrics/series?name=http_2xx_ratio&window=15m&step=15s&demo=healthy", "127.0.0.1:8088", goodHeaders())
	if rec.Code != http.StatusOK {
		t.Fatalf("series: got %d", rec.Code)
	}
	var body struct {
		Name       string   `json:"name"`
		Status     string   `json:"status"`
		LastDataAt *string  `json:"lastDataAt"`
		Points     [][2]any `json:"points"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("series bukan JSON: %v", err)
	}
	if body.Name != "http_2xx_ratio" || body.Status != "ok" || len(body.Points) == 0 {
		t.Fatalf("series tak lengkap: %+v", body)
	}
	if body.LastDataAt == nil || !validRFC3339(*body.LastDataAt) {
		t.Fatal("lastDataAt hilang/bukan RFC3339")
	}
}

// Tabel kasus semantik data (acceptance U1): 0 vs no_data vs stale vs down.
func TestClassifySemantics(t *testing.T) {
	now := time.Now()
	val := func(v float64, ago time.Duration) []promSample {
		return []promSample{{value: v, ts: now.Add(-ago)}}
	}
	cases := []struct {
		name       string
		samples    []promSample
		promErr    bool
		wantStatus string
		wantValue  bool // value non-nil?
	}{
		{"nol persen jujur", val(0, time.Second), false, "ok", true},
		{"vektor kosong bukan nol", nil, false, "no_data", false},
		{"NaN dari 0/0", val(math.NaN(), time.Second), false, "no_data", false},
		{"Inf", val(math.Inf(1), time.Second), false, "no_data", false},
		{"usang 60 dtk", val(0.9, 60*time.Second), false, "stale", false},
		{"batas usang 14 dtk masih ok", val(0.9, 14*time.Second), false, "ok", true},
		{"prometheus mati", val(0.9, time.Second), true, "source_down", false},
	}
	for _, c := range cases {
		var mv metricValue
		if c.promErr {
			mv = sourceDown("boom")
		} else {
			mv = classifyValue(c.samples, now)
		}
		if mv.Status != c.wantStatus {
			t.Fatalf("%s: status %q, want %q", c.name, mv.Status, c.wantStatus)
		}
		if (mv.Value != nil) != c.wantValue {
			t.Fatalf("%s: value nil=%v, want non-nil=%v", c.name, mv.Value == nil, c.wantValue)
		}
		if c.wantStatus == "stale" && mv.LastDataAt == nil {
			t.Fatalf("%s: stale tanpa lastDataAt", c.name)
		}
	}
}

func TestEventsSSE(t *testing.T) {
	s := demoServer()
	req := httptest.NewRequest("GET", "/api/v1/events?demo=healthy", nil)
	req.Host = "127.0.0.1:8088"
	ctx, cancel := context.WithTimeout(req.Context(), 300*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("SSE content-type %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: health") || !strings.Contains(body, "event: metrics") {
		t.Fatalf("SSE tanpa event awal:\n%s", body)
	}
}

func TestEventsSSEU4(t *testing.T) {
	s := testServer() // demo:true -> demoIncidents
	req := httptest.NewRequest("GET", "/api/v1/events", nil)
	req.Host = "127.0.0.1:8088"
	ctx, cancel := context.WithTimeout(req.Context(), 300*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "event: incident") {
		t.Fatalf("SSE tanpa event incident:\n%s", body)
	}
	if !strings.Contains(body, "event: approval") {
		t.Fatalf("SSE tanpa event approval:\n%s", body)
	}
	if !strings.Contains(body, demoIncidentID) {
		t.Fatalf("SSE incident tanpa id demo:\n%s", body)
	}
}
