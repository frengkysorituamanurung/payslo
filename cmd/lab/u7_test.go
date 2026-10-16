package main

// Test U7: observability (status, grafana, anotasi filter mode, prometheus,
// alertmanager, ntfy), diagnostik bebas secret, pengaturan ambang, degradasi
// anggun tanpa Docker, dan kontrak route vs openapi.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// ---------- status observability (demo: obs-full mati, degradasi anggun) ----------

func TestObserveStatusDemo(t *testing.T) {
	s := testServer() // demo:true
	rec := doJSON(t, s.handler(), "GET", "/api/v1/observe/status", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("observe/status: got %d", rec.Code)
	}
	var body struct {
		Services []struct {
			Name      string `json:"name"`
			Reachable bool   `json:"reachable"`
		} `json:"services"`
		DockerAvailable   bool            `json:"dockerAvailable"`
		DockerNote        string          `json:"dockerNote"`
		ComposeProfiles   map[string]bool `json:"composeProfiles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bukan JSON: %v", err)
	}
	if len(body.Services) == 0 {
		t.Fatal("services kosong")
	}
	byName := map[string]bool{}
	for _, svc := range body.Services {
		byName[svc.Name] = svc.Reachable
	}
	// Demo: loki/tempo MATI (obs-full off) tapi respons tetap 200.
	if byName["loki"] || byName["tempo"] {
		t.Fatal("demo: loki/tempo harus unreachable (obs-full mati)")
	}
	if !byName["grafana"] || !byName["prometheus"] {
		t.Fatal("demo: grafana/prometheus harus reachable")
	}
	if body.DockerAvailable {
		t.Fatal("demo: dockerAvailable harus false")
	}
	if !strings.Contains(body.DockerNote, "Docker tidak tersedia") {
		t.Fatalf("dockerNote harus jujur: %q", body.DockerNote)
	}
	if body.ComposeProfiles["obs-full"] {
		t.Fatal("demo: profil obs-full harus false")
	}
}

// ---------- anotasi: trainee tidak melihat chaos, mentor melihat semua ----------

func TestAnnotationsModeFilter(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	// Trainee (tanpa token mentor): chaos disaring.
	rec := doMentor(t, h, "GET", "/api/v1/observe/annotations", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("annotations trainee: got %d", rec.Code)
	}
	got := rec.Body.String()
	if strings.Contains(got, `"chaos"`) {
		t.Fatalf("trainee melihat anotasi chaos (spoiler): %s", got)
	}
	if !strings.Contains(got, "deploy") {
		t.Fatalf("trainee harus melihat anotasi deploy: %s", got)
	}
	// Mentor (dengan token): melihat semua termasuk chaos.
	token, _, err := s.mentor.unlock("1234")
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}
	rec2 := doMentor(t, h, "GET", "/api/v1/observe/annotations", "", token)
	if rec2.Code != http.StatusOK {
		t.Fatalf("annotations mentor: got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), `"chaos"`) {
		t.Fatalf("mentor harus melihat anotasi chaos: %s", rec2.Body.String())
	}
}

// ---------- diagnostik bebas secret ----------

func TestDiagnosticsSecretFree(t *testing.T) {
	s := testServer()
	rec := doJSON(t, s.handler(), "GET", "/api/v1/diagnostics", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("diagnostics: got %d", rec.Code)
	}
	raw := strings.ToLower(rec.Body.String())
	for _, forbidden := range []string{
		"password", "passwd", "api_key", "apikey", "bearer",
		"mentor_pin", "dd_api_key", "lab-only-", "authorization",
		"grafana_password", "webhook-secret", "webhook_secret",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("laporan mengandung pola sensitif %q:\n%s", forbidden, rec.Body.String())
		}
	}
	var body struct {
		Services []struct {
			Name string `json:"name"`
		} `json:"services"`
		Docker struct {
			Available bool   `json:"available"`
			Note      string `json:"note"`
		} `json:"docker"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("bukan JSON: %v", err)
	}
	if len(body.Services) == 0 {
		t.Fatal("services kosong")
	}
	if body.Docker.Available {
		t.Fatal("demo: docker.available harus false")
	}
	if !strings.Contains(body.Docker.Note, "Docker tidak tersedia") {
		t.Fatalf("docker.note harus jujur: %q", body.Docker.Note)
	}
}

// ---------- pengaturan: ambang data-sedikit ----------

func TestSettingsThreshold(t *testing.T) {
	s := testServer()
	h := s.handler()
	// Default: threshold tidak di-set (omitempty) — UI memakai 0.2.
	rec := doJSON(t, h, "GET", "/api/v1/settings", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("settings get: got %d", rec.Code)
	}
	// Set threshold valid.
	rec = doJSON(t, h, "PUT", "/api/v1/settings", `{"lowSampleThreshold":0.5}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("settings put threshold: got %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		LowSampleThreshold *float64 `json:"lowSampleThreshold"`
		TraineeName        string   `json:"traineeName"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("bukan JSON: %v", err)
	}
	if out.LowSampleThreshold == nil || *out.LowSampleThreshold != 0.5 {
		t.Fatalf("threshold tidak tersimpan: %s", rec.Body.String())
	}
	// Nama trainee tetap bisa di-set; threshold bertahan.
	rec = doJSON(t, h, "PUT", "/api/v1/settings", `{"traineeName":"sinta"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("settings put nama: got %d", rec.Code)
	}
	rec = doJSON(t, h, "GET", "/api/v1/settings", "", "")
	var out2 struct {
		LowSampleThreshold *float64 `json:"lowSampleThreshold"`
		TraineeName        string   `json:"traineeName"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out2)
	if out2.TraineeName != "sinta" {
		t.Fatalf("nama hilang: %s", rec.Body.String())
	}
	if out2.LowSampleThreshold == nil || *out2.LowSampleThreshold != 0.5 {
		t.Fatalf("threshold tertimpa oleh set nama: %s", rec.Body.String())
	}
	// Di luar rentang -> 400.
	for _, bad := range []string{`{"lowSampleThreshold":-1}`, `{"lowSampleThreshold":99}`} {
		rec = doJSON(t, h, "PUT", "/api/v1/settings", bad, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("threshold %s: got %d, want 400", bad, rec.Code)
		}
	}
}

// ---------- endpoint observe lain (demo fixtures) ----------

func TestObserveEndpointsDemo(t *testing.T) {
	s := testServer()
	h := s.handler()
	for _, target := range []string{
		"/api/v1/observe/grafana",
		"/api/v1/observe/prometheus/alerts",
		"/api/v1/observe/prometheus/rules",
		"/api/v1/observe/alertmanager",
		"/api/v1/observe/ntfy/history",
	} {
		rec := doJSON(t, h, "GET", target, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: got %d", target, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
			t.Fatalf("GET %s: content-type %q", target, ct)
		}
	}
	// Grafana embed: kiosk URL + dashboard terprovision.
	rec := doJSON(t, h, "GET", "/api/v1/observe/grafana", "", "")
	var g struct {
		KioskURL         string `json:"kioskUrl"`
		EmbeddingAllowed bool   `json:"embeddingAllowed"`
		Dashboards       []struct {
			Title string `json:"title"`
			UID   string `json:"uid"`
		} `json:"dashboards"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &g)
	if !strings.Contains(g.KioskURL, "kiosk") {
		t.Fatalf("kioskUrl tanpa ?kiosk: %q", g.KioskURL)
	}
	if len(g.Dashboards) == 0 {
		t.Fatal("dashboards kosong")
	}
}

// ---------- kontrak: route U7 terdaftar ----------

func TestU7RoutesRegistered(t *testing.T) {
	s := testServer()
	want := []string{
		"get /api/v1/observe/status",
		"get /api/v1/observe/grafana",
		"get /api/v1/observe/annotations",
		"get /api/v1/observe/prometheus/alerts",
		"get /api/v1/observe/prometheus/rules",
		"get /api/v1/observe/alertmanager",
		"get /api/v1/observe/ntfy/history",
		"get /api/v1/diagnostics",
	}
	have := map[string]bool{}
	for _, r := range s.routes {
		have[strings.ToLower(r)] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("route tidak terdaftar: %s", w)
		}
	}
}
