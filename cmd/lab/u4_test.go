package main

// Test U4: runbook load + enforcement, postmortem render/save, settings,
// alur demo approval (request -> approve -> executed, reject, double),
// handler HTTP demo, dan keamanan ID.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- runbook ----------

func TestValidateRunbookDoc(t *testing.T) {
	ok := runbookDoc{
		APIVersion: "payslo/v1", Kind: "Runbook", Name: "x", Slo: "payment-success-rate",
		Steps: []runbookStep{
			{Name: "d1", Kind: "diagnostic", Run: runSpec{Type: "promql", Query: "payment_success_ratio"}},
			{Name: "m1", Kind: "manual"},
			{Name: "r1", Kind: "remediation", Impact: "dampak", RequiresApproval: boolptr(true),
				Run: runSpec{Type: "lab_routing", Disable: []string{"bca"}}},
		},
	}
	if msg := validateRunbookDoc(&ok); msg != "" {
		t.Fatalf("runbook valid ditolak: %s", msg)
	}
	cases := []struct {
		name   string
		mutate func(*runbookDoc)
	}{
		{"remediation tanpa approval", func(d *runbookDoc) { d.Steps[2].RequiresApproval = boolptr(false) }},
		{"remediation tanpa impact", func(d *runbookDoc) { d.Steps[2].Impact = "" }},
		{"remediation executor salah", func(d *runbookDoc) { d.Steps[2].Run.Type = "http" }},
		{"diagnostic promql bebas", func(d *runbookDoc) { d.Steps[0].Run.Query = "up{job='x'}" }},
		{"diagnostic http non-loopback", func(d *runbookDoc) {
			d.Steps[0].Run = runSpec{Type: "http", Method: "GET", URL: "http://example.com/x"}
		}},
		{"diagnostic http POST", func(d *runbookDoc) {
			d.Steps[0].Run = runSpec{Type: "http", Method: "POST", URL: "http://127.0.0.1:4001/x"}
		}},
		{"kind asing", func(d *runbookDoc) { d.Steps[1].Kind = "magic" }},
	}
	for _, c := range cases {
		doc := ok
		doc.Steps = append([]runbookStep{}, ok.Steps...)
		c.mutate(&doc)
		if msg := validateRunbookDoc(&doc); msg == "" {
			t.Fatalf("%s LOLOS (harus ditolak)", c.name)
		}
	}
}

func boolptr(b bool) *bool { return &b }

func TestRealRunbookLoads(t *testing.T) {
	store := &runbookStore{dir: filepath.Join("..", "..", "payslo-workspace", "runbooks")}
	view := store.forSLO("payment-success-rate")
	if !view.Present {
		t.Fatal("runbook payment-success-rate tak ditemukan")
	}
	if len(view.Steps) != 5 {
		t.Fatalf("steps = %d, want 5", len(view.Steps))
	}
	rem := view.Steps[4]
	if rem.Kind != "remediation" || !rem.RequiresApproval || rem.Action != "disable provider bca" {
		t.Fatalf("langkah remediation salah: %+v", rem)
	}
	if _, ok := view.stepForAction("disable provider bca"); !ok {
		t.Fatal("stepForAction tak menemukan aksi remediation")
	}
}

func TestValidIncidentID(t *testing.T) {
	for _, good := range []string{"inc-abcdef0123456789", "inc-9f2c4a7e1b3d5a6c"} {
		if !validIncidentID(good) {
			t.Fatalf("%q ditolak", good)
		}
	}
	for _, bad := range []string{"", "inc-", "../x", "/tmp/x", "inc-A B", "x", strings.Repeat("a", 65)} {
		if validIncidentID(bad) {
			t.Fatalf("%q DITERIMA", bad)
		}
	}
}

// ---------- postmortem ----------

func TestRenderPostmortem(t *testing.T) {
	inc := incidentSummary{ID: "inc-x", Title: "T", Slo: "s", Service: "svc", Status: "open", Severity: "page"}
	doc := blankPostmortem("inc-x")
	doc.Sections.Summary = "ringkasan"
	doc.Sections.Actions = []string{"a1", "a2"}
	md := renderPostmortem(inc, doc)
	for _, want := range []string{
		"# Postmortem T (inc-x)", "- Status: draft",
		"## Ringkasan", "ringkasan",
		"## Dampak", "## Kronologi", "## Akar masalah",
		"## Yang berjalan baik", "## Yang berjalan buruk",
		"## Action items", "- [ ] a1", "- [ ] a2",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown tanpa %q:\n%s", want, md)
		}
	}
	// Bagian kosong jujur (placeholder, bukan hilang).
	if !strings.Contains(renderPostmortem(inc, blankPostmortem("inc-x")), "_belum diisi_") {
		t.Fatal("bagian kosong harus placeholder")
	}
}

func TestPostmortemStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	st := &postmortemStore{dir: dir}
	inc := incidentSummary{ID: "inc-abcdef0123456789", Title: "T"}
	got, err := st.Get(inc, "inc-abcdef0123456789")
	if err != nil || got.Exists {
		t.Fatalf("baru harus blank: %+v %v", got, err)
	}
	saved, err := st.Save("inc-abcdef0123456789", "published",
		postmortemSections{Summary: "s", Actions: []string{"a"}}, "sinta")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "published" || !strings.Contains(saved.Markdown, "") {
		t.Fatalf("save aneh: %+v", saved)
	}
	again, err := st.Get(inc, "inc-abcdef0123456789")
	if err != nil || !again.Exists || again.Sections.Summary != "s" {
		t.Fatalf("roundtrip gagal: %+v %v", again, err)
	}
	if _, err := st.Save("inc-abcdef0123456789", "bogus", postmortemSections{}, "x"); err == nil {
		t.Fatal("status bogus DITERIMA")
	}
	if _, err := st.Get(inc, "../evil"); err == nil {
		t.Fatal("ID traversal DITERIMA")
	}
}

// ---------- settings ----------

func TestSettingsStore(t *testing.T) {
	dir := t.TempDir()
	st := &settingsStore{path: filepath.Join(dir, "settings.json")}
	if got := st.Get(); got.TraineeName != "" {
		t.Fatalf("awal harus kosong: %+v", got)
	}
	if _, err := st.Set(""); err == nil {
		t.Fatal("nama kosong DITERIMA")
	}
	if _, err := st.Set("a\nb"); err == nil {
		t.Fatal("nama ber-newline DITERIMA")
	}
	out, err := st.Set("  sinta  ")
	if err != nil || out.TraineeName != "sinta" {
		t.Fatalf("set gagal: %+v %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatalf("tak tertulis: %v", err)
	}
}

// ---------- alur demo ----------

func TestDemoApprovalFlow(t *testing.T) {
	ctx := context.Background()
	d := newDemoIncidents()
	// Request.
	ap, err := d.RequestApproval(ctx, demoIncidentID, "disable provider bca", 4, "sinta")
	if err != nil || ap.Status != "pending" {
		t.Fatalf("request gagal: %+v %v", ap, err)
	}
	// Dedup: request sama -> baris sama.
	ap2, err := d.RequestApproval(ctx, demoIncidentID, "disable provider bca", 4, "sinta")
	if err != nil || ap2.ID != ap.ID || !ap2.Deduped {
		t.Fatalf("dedup gagal: %+v %v", ap2, err)
	}
	// Reject tanpa alasan -> ditolak (acceptance).
	if _, err := d.DecideApproval(ctx, ap.ID, "rejected", "sinta", ""); err == nil {
		t.Fatal("reject tanpa alasan DITERIMA")
	}
	// Approve tanpa approver & tanpa settings -> ditolak.
	if _, err := d.DecideApproval(ctx, ap.ID, "approved", "", ""); err == nil {
		t.Fatal("approve tanpa identitas DITERIMA")
	}
	// Set settings -> approve via identitas pengaturan.
	if _, err := d.SetSettings("sinta"); err != nil {
		t.Fatal(err)
	}
	decided, err := d.DecideApproval(ctx, ap.ID, "approved", "", "")
	if err != nil || decided.Status != "approved" || decided.DecidedBy == nil || *decided.DecidedBy != "sinta" {
		t.Fatalf("approve gagal: %+v %v", decided, err)
	}
	// Keputusan ganda -> konflik aman (acceptance 409).
	if _, err := d.DecideApproval(ctx, ap.ID, "rejected", "sinta", "x"); err == nil {
		t.Fatal("keputusan ganda DITERIMA")
	} else if got := statusOf(err); got != http.StatusConflict {
		t.Fatalf("keputusan ganda harus 409, got %d (%v)", got, err)
	}
	// Timeline mencatat request + approve + eksekusi.
	detail, err := d.Get(ctx, demoIncidentID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, e := range detail.Timeline {
		kinds[e.Kind] = true
	}
	for _, want := range []string{"opened", "approval_requested", "approval_approved", "remediation"} {
		if !kinds[want] {
			t.Fatalf("timeline tanpa %q: %+v", want, detail.Timeline)
		}
	}
	if !d.routingB {
		t.Fatal("efek remediation demo (bca disabled) tak tercatat")
	}
	// RunStep: diagnostic ok, remediation ditolak langsung (tak ada jalan pintas).
	if _, err := d.RunStep(ctx, demoIncidentID, 0); err != nil {
		t.Fatalf("diagnostic gagal: %v", err)
	}
	if _, err := d.RunStep(ctx, demoIncidentID, 4); err == nil {
		t.Fatal("remediation langsung DIIZINKAN (jalan pintas!)")
	}
}

// ---------- handler HTTP demo ----------

func TestIncidentHandlersDemo(t *testing.T) {
	s := testServer() // demo:true
	h := s.handler()
	// List.
	rec := doJSON(t, h, "GET", "/api/v1/incidents?status=open", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), demoIncidentID) {
		t.Fatalf("list: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Detail + XSS verbatim setelah decode (Go meng-escape HTML di JSON
	// mentah — aman & transparan; UI meng-escape saat render).
	rec = doJSON(t, h, "GET", "/api/v1/incidents/"+demoIncidentID, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: got %d", rec.Code)
	}
	var detail struct {
		Alerts []struct {
			Payload map[string]any `json:"payload"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("detail bukan JSON: %v", err)
	}
	foundXSS := false
	for _, a := range detail.Alerts {
		labels, _ := a.Payload["labels"].(map[string]any)
		if labels["alertname"] == "<script>alert(1)</script>" {
			foundXSS = true
		}
	}
	if !foundXSS {
		t.Fatal("payload XSS harus verbatim setelah decode (UI yang meng-escape)")
	}
	if !strings.Contains(rec.Body.String(), `"kind":"remediation"`) {
		t.Fatal("detail tanpa langkah remediation")
	}
	// Unknown -> 404; traversal -> 404.
	rec = doJSON(t, h, "GET", "/api/v1/incidents/inc-tidak-ada", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: got %d, want 404", rec.Code)
	}
	rec = doJSON(t, h, "GET", "/api/v1/incidents/..%2F..%2Fetc", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal: got %d, want 404", rec.Code)
	}
	// Catatan kosong -> 400; berisi -> 201 + timeline.
	rec = doJSON(t, h, "POST", "/api/v1/incidents/"+demoIncidentID+"/notes", `{"text":"  "}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("note kosong: got %d, want 400", rec.Code)
	}
	rec = doJSON(t, h, "POST", "/api/v1/incidents/"+demoIncidentID+"/notes", `{"text":"observasi <b>penting</b>"}`, "sinta")
	if rec.Code != http.StatusCreated {
		t.Fatalf("note: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Approval: request -> approve (settings dulu) -> approved.
	rec = doJSON(t, h, "PUT", "/api/v1/settings", `{"traineeName":"sinta"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("settings: got %d", rec.Code)
	}
	rec = doJSON(t, h, "POST", "/api/v1/incidents/"+demoIncidentID+"/approvals", `{"action":"disable provider bca","step":4}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("request approval: got %d (%s)", rec.Code, rec.Body.String())
	}
	aid := jsonField(t, rec.Body.Bytes(), "approval", "id")
	// Aksi bebas ditolak (bukan langkah runbook).
	rec = doJSON(t, h, "POST", "/api/v1/incidents/"+demoIncidentID+"/approvals", `{"action":"rm -rf /","step":9}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("aksi bebas: got %d, want 400", rec.Code)
	}
	rec = doJSON(t, h, "POST", "/api/v1/approvals/"+aid+"/decision", `{"decision":"approved","by":"sinta"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Postmortem: blank -> save -> markdown valid.
	rec = doJSON(t, h, "GET", "/api/v1/incidents/"+demoIncidentID+"/postmortem", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "# Postmortem") {
		t.Fatalf("postmortem blank: got %d", rec.Code)
	}
	rec = doJSON(t, h, "PUT", "/api/v1/incidents/"+demoIncidentID+"/postmortem",
		`{"status":"published","sections":{"summary":"ringkasan","impact":"dampak","timeline":"kronologi","rootCause":"bca down","wentWell":"deteksi cepat","wentWrong":"tak ada","actions":["tambah alert"]}}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("postmortem save: got %d (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"# Postmortem", "## Ringkasan", "## Action items", "- [ ] tambah alert"} {
		if !strings.Contains(body, want) {
			t.Fatalf("markdown tanpa %q", want)
		}
	}
	// RunStep remediation langsung -> 409 (tak ada jalan pintas).
	rec = doJSON(t, h, "POST", "/api/v1/incidents/"+demoIncidentID+"/runbook/steps/4/run", ``, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("run remediation langsung: got %d, want 409", rec.Code)
	}
	rec = doJSON(t, h, "POST", "/api/v1/incidents/"+demoIncidentID+"/runbook/steps/0/run", ``, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("run diagnostic: got %d (%s)", rec.Code, rec.Body.String())
	}
}

func jsonField(t *testing.T, body []byte, keys ...string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("bukan JSON: %v", err)
	}
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("bukan objek di %q", k)
		}
		v = m[k]
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("field %v bukan string", keys)
	}
	return s
}
