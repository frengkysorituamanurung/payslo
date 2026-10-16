package main

// Test U6: mode mentor (PIN + rate limit), timeline merge dua sumber,
// snapshot save/chaos/load, progres belajar tahan restart, sanitasi markdown,
// penegakan 403 trainee.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- sanitasi markdown (acceptance: <script>, event handler, javascript:, iframe) ----------

func TestMarkdownSanitized(t *testing.T) {
	md := strings.Join([]string{
		"# Judul",
		"Normal **tebal** dan *miring* dan `kode`.",
		"<script>alert('xss')</script>",
		`<img src=x onerror="alert(1)">`,
		`<iframe src="https://evil.com"></iframe>`,
		`<a href="javascript:alert(2)">klik</a>`,
		"[link baik](https://example.com) dan [link jahat](javascript:alert(3))",
		"1. item satu",
		"2. item dua",
	}, "\n")
	html := renderMarkdown(md)
	// Semua HTML mentah harus di-escape: tidak ada tag < yang lolos.
	for _, bad := range []string{"<script", "<iframe", "<img", "<a href=\"javascript:"} {
		if strings.Contains(html, bad) {
			t.Fatalf("HTML masah mengandung %q:\n%s", bad, html)
		}
	}
	// Escape harus terlihat sebagai entitas.
	if !strings.Contains(html, "&lt;script") {
		t.Fatal("<script> harus di-escape menjadi &lt;script")
	}
	// Event handler onerror hanya boleh muncul dalam bentuk escaped
	// (onerror=&quot;), tidak pernah sebagai atribut live (onerror=").
	if strings.Contains(html, `onerror="`) {
		t.Fatalf("event handler onerror tidak boleh menjadi atribut live:\n%s", html)
	}
	// javascript: tidak boleh muncul sebagai href live.
	if strings.Contains(html, "href=\"javascript:") {
		t.Fatalf("href javascript: harus dinetralkan:\n%s", html)
	}
	// Link baik tetap berfungskan.
	if !strings.Contains(html, `<a href="https://example.com">`) {
		t.Fatal("link https harus tetap dirender")
	}
	if !strings.Contains(html, "link jahat") {
		t.Fatal("teks link jahat harus tetap tampil sebagai teks")
	}
	// Struktur dasar tetap ada.
	for _, want := range []string{"<h1>", "<strong>", "<em>", "<code>", "<ol>", "<li>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML tanpa %q:\n%s", want, html)
		}
	}
}

// ---------- gate PIN + rate limit ----------

func mentorTestServer(t *testing.T, pin string) *server {
	t.Helper()
	return newServer(serverConfig{
		demo:       true,
		webDir:     "testdata/web",
		token:      testToken,
		port:       testPort,
		mentorPIN:  pin,
		repoRoot:   filepath.Join("..", ".."),
		consoleDir: t.TempDir(),
	})
}

func doMentor(t *testing.T, h http.Handler, method, target, body, mentorToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = "127.0.0.1:8088"
	req.Header.Set("Content-Type", "application/json")
	if method != "GET" {
		req.Header.Set("Origin", "http://127.0.0.1:8088")
		req.Header.Set("X-Lab-Token", testToken)
	}
	if mentorToken != "" {
		req.Header.Set("X-Mentor-Token", mentorToken)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMentorGateRateLimit(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	// 5x salah -> 401.
	for i := 0; i < 5; i++ {
		rec := doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"salah"}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("percobaan %d: got %d, want 401", i+1, rec.Code)
		}
	}
	// Ke-6: terkunci -> 429 + Retry-After.
	rec := doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("setelah 5 salah: got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 harus membawa Retry-After")
	}
	// White-box: percepat berakhirnya jeda agar pengujian PIN benar lanjut.
	s.mentor.lockedUntil = time.Now().Add(-time.Second)
	// Terima PIN benar -> 200 + token.
	rec = doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("PIN benar: got %d, want 200", rec.Code)
	}
	var body struct {
		Ok    bool   `json:"ok"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.Ok || body.Token == "" {
		t.Fatalf("unlock gagal: %v (%s)", err, rec.Body.String())
	}
	// Token dipakai di endpoint mentor.
	rec = doMentor(t, h, "GET", "/api/v1/mentor/timeline", "", body.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline dengan token: got %d, want 200", rec.Code)
	}
	// Lock -> token dicabut.
	rec = doMentor(t, h, "POST", "/api/v1/mentor/lock", "", body.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("lock: got %d", rec.Code)
	}
	rec = doMentor(t, h, "GET", "/api/v1/mentor/timeline", "", body.Token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("timeline setelah lock: got %d, want 403", rec.Code)
	}
}

func TestMentorDisabledWhenNoPIN(t *testing.T) {
	s := mentorTestServer(t, "")
	h := s.handler()
	rec := doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tanpa MENTOR_PIN: got %d, want 403", rec.Code)
	}
}

// ---------- penegakan 403 trainee ----------

func TestMentorTraineeForbidden(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	// Tanpa token: semua endpoint mentor -> 403.
	for _, target := range []string{
		"/api/v1/mentor/timeline",
		"/api/v1/mentor/snapshots",
		"/api/v1/mentor/sessions",
		"/api/v1/mentor/session-summary?scenario=01",
		"/api/v1/mentor/assessment?scenario=01",
		"/api/v1/mentor/export?scenario=01",
	} {
		rec := doMentor(t, h, "GET", target, "", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("GET %s tanpa token: got %d, want 403", target, rec.Code)
		}
	}
	// Token salah -> 403.
	rec := doMentor(t, h, "GET", "/api/v1/mentor/timeline", "", "token-ngawur")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("token salah: got %d, want 403", rec.Code)
	}
	// Web /mentor untuk trainee -> 403.
	rec = doMentor(t, h, "GET", "/mentor", "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /mentor trainee: got %d, want 403", rec.Code)
	}
	// Unlock -> token -> /mentor bisa diakses.
	rec = doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	var body struct{ Token string `json:"token"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	rec = doMentor(t, h, "GET", "/mentor", "", body.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /mentor mentor: got %d, want 200", rec.Code)
	}
	// Session melaporkan mode mentor dengan token.
	rec = doMentor(t, h, "GET", "/api/v1/session", "", body.Token)
	var sess struct{ Mode string `json:"mode"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	if sess.Mode != "mentor" {
		t.Fatalf("mode dengan token: got %q, want mentor", sess.Mode)
	}
	// Tanpa token: trainee.
	rec = doMentor(t, h, "GET", "/api/v1/session", "", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	if sess.Mode != "trainee" {
		t.Fatalf("mode tanpa token: got %q, want trainee", sess.Mode)
	}
}

// ---------- timeline merge dua sumber (data berselang-seling) ----------

func TestMergeTimelineInterleaved(t *testing.T) {
	// Data berselang-seling: payslo dan lab saling bergantian.
	payslo := []mentorTimelineEntry{
		{TS: "2026-10-16T07:00:01Z", Source: "payslo", Actor: "payslo", Action: "incident.open", Detail: "p1"},
		{TS: "2026-10-16T07:00:03Z", Source: "payslo", Actor: "payslo", Action: "incident.note", Detail: "p2"},
		{TS: "2026-10-16T07:00:05Z", Source: "payslo", Actor: "payslo", Action: "approval.decide", Detail: "p3"},
	}
	lab := []mentorTimelineEntry{
		{TS: "2026-10-16T07:00:00Z", Source: "lab", Actor: "trainee", Action: "routing.set", Detail: "l0"},
		{TS: "2026-10-16T07:00:02Z", Source: "lab", Actor: "trainee", Action: "traffic.set", Detail: "l1"},
		{TS: "2026-10-16T07:00:04Z", Source: "lab", Actor: "trainee", Action: "answer", Detail: "l2"},
		{TS: "2026-10-16T07:00:06Z", Source: "lab", Actor: "trainee", Action: "chaos.reset", Detail: "l3"},
	}
	merged := mergeTimeline(payslo, lab, 100)
	if len(merged) != 7 {
		t.Fatalf("merged = %d, want 7", len(merged))
	}
	// Urut naik per ts.
	for i := 1; i < len(merged); i++ {
		if merged[i-1].TS > merged[i].TS {
			t.Fatalf("urutan salah di %d: %s > %s", i, merged[i-1].TS, merged[i].TS)
		}
	}
	// Interleave benar: l0 p1 l1 p2 l2 p3 l3.
	wantOrder := []string{"l0", "p1", "l1", "p2", "l2", "p3", "l3"}
	for i, w := range wantOrder {
		if merged[i].Detail != w {
			t.Fatalf("posisi %d: got %q, want %q", i, merged[i].Detail, w)
		}
	}
	// Limit: 3 entri terakhir.
	limited := mergeTimeline(payslo, lab, 3)
	if len(limited) != 3 || limited[0].Detail != "l2" {
		t.Fatalf("limit 3: got %v", limited)
	}
}

func TestMentorTimelineEndpoint(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	// Unlock dulu (tercatat di audit dengan TS sekarang).
	rec := doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	var body struct{ Token string `json:"token"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	// Audit lab: catat beberapa entri dengan TS deterministik.
	s.audit.record(auditEntry{TS: "2026-10-16T07:00:00Z", Actor: "trainee", Action: "routing.set", Target: "gateway", Detail: "bca=false"})
	s.audit.record(auditEntry{TS: "2026-10-16T07:00:05Z", Actor: "trainee", Action: "answer", Target: "q1", Detail: "jawaban"})
	// Timeline.
	rec = doMentor(t, h, "GET", "/api/v1/mentor/timeline", "", body.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline: got %d", rec.Code)
	}
	var tl struct {
		Entries []mentorTimelineEntry `json:"entries"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &tl)
	if len(tl.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(tl.Entries))
	}
	// Urut naik per ts (TS unlock = now, posisi relatifnya tak penting).
	for i := 1; i < len(tl.Entries); i++ {
		if tl.Entries[i-1].TS > tl.Entries[i].TS {
			t.Fatalf("urutan ts salah di %d: %s > %s", i, tl.Entries[i-1].TS, tl.Entries[i].TS)
		}
	}
	// routing.set harus sebelum answer; keduanya source lab.
	idxRouting, idxAnswer := -1, -1
	for i, e := range tl.Entries {
		if e.Action == "routing.set" {
			idxRouting = i
		}
		if e.Action == "answer" {
			idxAnswer = i
		}
	}
	if idxRouting < 0 || idxAnswer < 0 || idxRouting >= idxAnswer {
		t.Fatalf("routing.set harus sebelum answer: %+v", tl.Entries)
	}
	if tl.Entries[idxRouting].Source != "lab" || tl.Entries[idxAnswer].Source != "lab" {
		t.Fatalf("source harus lab: %+v", tl.Entries)
	}
}

// ---------- snapshot save -> chaos -> load ----------

func TestSnapshotSaveChaosLoad(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	// Unlock.
	rec := doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	var body struct{ Token string `json:"token"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	tok := body.Token
	// Kondisi awal: traffic 2, routing default.
	rec = doMentor(t, h, "GET", "/api/v1/traffic", "", tok)
	var beforeTraffic struct {
		RatePerSec *float64 `json:"ratePerSec"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &beforeTraffic)
	if beforeTraffic.RatePerSec == nil || *beforeTraffic.RatePerSec != 2 {
		t.Fatalf("traffic awal: got %+v, want 2", beforeTraffic.RatePerSec)
	}
	// Save snapshot.
	rec = doMentor(t, h, "POST", "/api/v1/mentor/snapshot/save", `{"name":"awal"}`, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("save: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Chaos: ubah traffic ke 10.
	rec = doMentor(t, h, "PUT", "/api/v1/traffic", `{"ratePerSec":10}`, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("chaos traffic: got %d", rec.Code)
	}
	rec = doMentor(t, h, "GET", "/api/v1/traffic", "", tok)
	var chaosTraffic struct {
		RatePerSec *float64 `json:"ratePerSec"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &chaosTraffic)
	if chaosTraffic.RatePerSec == nil || *chaosTraffic.RatePerSec != 10 {
		t.Fatalf("traffic chaos: got %+v, want 10", chaosTraffic.RatePerSec)
	}
	// Load snapshot.
	rec = doMentor(t, h, "POST", "/api/v1/mentor/snapshot/load", `{"name":"awal"}`, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("load: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Bukti lewat API: traffic kembali ke 2.
	rec = doMentor(t, h, "GET", "/api/v1/traffic", "", tok)
	var afterTraffic struct {
		RatePerSec *float64 `json:"ratePerSec"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &afterTraffic)
	if afterTraffic.RatePerSec == nil || *afterTraffic.RatePerSec != 2 {
		t.Fatalf("traffic setelah load: got %+v, want 2 (kondisi awal)", afterTraffic.RatePerSec)
	}
	// Snapshot tidak ditemukan -> 404.
	rec = doMentor(t, h, "POST", "/api/v1/mentor/snapshot/load", `{"name":"ngawur"}`, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("load ngawur: got %d, want 404", rec.Code)
	}
	// Daftar snapshot memuat "awal".
	rec = doMentor(t, h, "GET", "/api/v1/mentor/snapshots", "", tok)
	var list struct {
		Snapshots []struct {
			Name string `json:"name"`
		} `json:"snapshots"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Snapshots) != 1 || list.Snapshots[0].Name != "awal" {
		t.Fatalf("list snapshots: %+v", list.Snapshots)
	}
}

// ---------- progres belajar tahan restart ----------

func TestLearnProgressPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s := newServer(serverConfig{
		demo:      true,
		webDir:    "testdata/web",
		token:     testToken,
		port:      testPort,
		mentorPIN: "1234",
		repoRoot:  filepath.Join("..", ".."),
		consoleDir: dir,
	})
	h := s.handler()
	// Simpan progres.
	rec := doMentor(t, h, "PUT", "/api/v1/learn/progress", `{"module":"day-01","key":"aktivitas-1","done":true}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("put progress: got %d", rec.Code)
	}
	// "Restart": server baru, DB sama.
	s2 := newServer(serverConfig{
		demo:      true,
		webDir:    "testdata/web",
		token:     testToken,
		port:      testPort,
		mentorPIN: "1234",
		repoRoot:  filepath.Join("..", ".."),
		consoleDir: dir,
	})
	h2 := s2.handler()
	rec = doMentor(t, h2, "GET", "/api/v1/learn/progress", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get progress: got %d", rec.Code)
	}
	var prog struct {
		Items []struct {
			Module string `json:"module"`
			Key    string `json:"key"`
			Done   bool   `json:"done"`
		} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &prog)
	if len(prog.Items) != 1 || prog.Items[0].Module != "day-01" || prog.Items[0].Key != "aktivitas-1" || !prog.Items[0].Done {
		t.Fatalf("progres tidak bertahan: %+v", prog.Items)
	}
}

// ---------- kurikulum + glosarium ----------

func TestCurriculumRendered(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	rec := doMentor(t, h, "GET", "/api/v1/learn/curriculum", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("curriculum: got %d", rec.Code)
	}
	var curr struct {
		Modules []struct {
			Day   string `json:"day"`
			Title string `json:"title"`
			HTML  string `json:"html"`
			Items []struct {
				Key    string `json:"key"`
				Label  string `json:"label"`
				Section string `json:"section"`
			} `json:"items"`
		} `json:"modules"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &curr)
	if len(curr.Modules) != 5 {
		t.Fatalf("modules = %d, want 5", len(curr.Modules))
	}
	// Modul pertama: day-01, ada checklist.
	m1 := curr.Modules[0]
	if m1.Day != "day-01" || m1.Title == "" {
		t.Fatalf("modul 1 aneh: %+v", m1)
	}
	if !strings.Contains(m1.HTML, "<h1>") {
		t.Fatal("HTML tanpa heading")
	}
	if len(m1.Items) == 0 {
		t.Fatal("modul 1 tanpa checklist")
	}
	// Glosarium.
	rec = doMentor(t, h, "GET", "/api/v1/learn/glossary", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("glossary: got %d", rec.Code)
	}
	var glos struct {
		Terms []struct {
			Term       string `json:"term"`
			Definition string `json:"definition"`
		} `json:"terms"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &glos)
	if len(glos.Terms) < 5 {
		t.Fatalf("glossary = %d, want >= 5", len(glos.Terms))
	}
}

// ---------- sesi + penilaian + ekspor ----------

func TestMentorSessionSummaryAndAssessment(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	// Buat sesi palsu.
	sessDir := filepath.Join(s.cfg.repoRoot, "payslo-workspace", "lab-sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessJSON := `{
		"scenario_id": "01",
		"started_at": 1791936000,
		"ended_at": 1791936300,
		"hints_used": ["h1"],
		"answers": {"q1": "jawaban pertama"},
		"actions": [
			{"ts": 1791936060, "cmd": "routing", "detail": "bca=false"},
			{"ts": 1791936120, "cmd": "answer", "detail": "q1"}
		]
	}`
	if err := os.WriteFile(filepath.Join(sessDir, "01.json"), []byte(sessJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	// Unlock.
	rec := doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	var body struct{ Token string `json:"token"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	tok := body.Token
	// Session summary.
	rec = doMentor(t, h, "GET", "/api/v1/mentor/session-summary?scenario=01", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("session-summary: got %d (%s)", rec.Code, rec.Body.String())
	}
	var sum struct {
		ScenarioID   string `json:"scenarioId"`
		TTDSecs      *int64 `json:"ttdSecs"`
		ActionCount  int    `json:"actionCount"`
		AnswerCount  int    `json:"answerCount"`
		HintsUsed    int    `json:"hintsUsed"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sum)
	if sum.ScenarioID != "01" || sum.ActionCount != 2 || sum.AnswerCount != 1 || sum.HintsUsed != 1 {
		t.Fatalf("summary salah: %+v", sum)
	}
	if sum.TTDSecs == nil || *sum.TTDSecs != 60 {
		t.Fatalf("TTD = %v, want 60", sum.TTDSecs)
	}
	// Simpan penilaian.
	rec = doMentor(t, h, "PUT", "/api/v1/mentor/assessment?scenario=01",
		`{"communicationScore":8,"communicationNote":"kabar jelas","postmortemScore":7,"postmortemNote":"lengkap"}`, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("assessment put: got %d", rec.Code)
	}
	// Skor di luar rentang -> 400.
	rec = doMentor(t, h, "PUT", "/api/v1/mentor/assessment?scenario=01",
		`{"communicationScore":11}`, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("skor 11: got %d, want 400", rec.Code)
	}
	// Ekspor markdown.
	rec = doMentor(t, h, "GET", "/api/v1/mentor/export?scenario=01&format=markdown", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("export md: got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Laporan Sesi Mentor") {
		t.Fatal("export markdown tanpa judul")
	}
	if !strings.Contains(rec.Body.String(), "Komunikasi (0-10): 8") {
		t.Fatal("export markdown tanpa skor komunikasi")
	}
	// Ekspor JSON.
	rec = doMentor(t, h, "GET", "/api/v1/mentor/export?scenario=01&format=json", "", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("export json: got %d", rec.Code)
	}
	var exported struct {
		ScenarioID string `json:"scenarioId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &exported)
	if exported.ScenarioID != "01" {
		t.Fatal("export json salah")
	}
	// Sesi tidak ditemukan -> 404.
	rec = doMentor(t, h, "GET", "/api/v1/mentor/session-summary?scenario=99", "", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sesi 99: got %d, want 404", rec.Code)
	}
}

// ---------- inject traffic ----------

func TestMentorInjectTraffic(t *testing.T) {
	s := mentorTestServer(t, "1234")
	h := s.handler()
	rec := doMentor(t, h, "POST", "/api/v1/mentor/unlock", `{"pin":"1234"}`, "")
	var body struct{ Token string `json:"token"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	// Inject x3 (dari 2 ke 6).
	rec = doMentor(t, h, "POST", "/api/v1/mentor/inject/traffic", `{"ratePerSec":6}`, body.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("inject: got %d", rec.Code)
	}
	var st struct {
		RatePerSec *float64 `json:"ratePerSec"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.RatePerSec == nil || *st.RatePerSec != 6 {
		t.Fatalf("rate = %v, want 6", st.RatePerSec)
	}
	// Skor di luar rentang -> 400.
	rec = doMentor(t, h, "POST", "/api/v1/mentor/inject/traffic", `{"ratePerSec":200}`, body.Token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rate 200: got %d, want 400", rec.Code)
	}
}

// ---------- kontrak vs openapi.yaml (route baru terdaftar) ----------

func TestU6RoutesRegistered(t *testing.T) {
	s := mentorTestServer(t, "1234")
	want := []string{
		"post /api/v1/mentor/unlock",
		"post /api/v1/mentor/lock",
		"get /api/v1/mentor/timeline",
		"post /api/v1/mentor/inject/traffic",
		"get /api/v1/mentor/snapshots",
		"post /api/v1/mentor/snapshot/save",
		"post /api/v1/mentor/snapshot/load",
		"get /api/v1/mentor/sessions",
		"get /api/v1/mentor/session-summary",
		"get /api/v1/mentor/assessment",
		"put /api/v1/mentor/assessment",
		"get /api/v1/mentor/export",
		"get /api/v1/learn/curriculum",
		"get /api/v1/learn/glossary",
		"get /api/v1/learn/progress",
		"put /api/v1/learn/progress",
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

// ---------- waktu helper ----------

func init() {
	_ = time.Now
}
