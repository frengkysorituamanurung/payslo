package main

// Test U3: keamanan path editor, validasi berlokasi, parser CLI, budget,
// alur demo, dan handler HTTP (termasuk no-op apply kedua).

import (
	"context"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- keamanan path ----------

func TestResolveSLOPathRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	bad := []string{
		"", ".", "..", "../x.yml", "../../etc/passwd", "/abs/path.yml",
		"a/b.yml", `a\b.yml`, "x.txt", "x.YML", "x.yaml ",
		"UPPER.YML", "sp ace.yml", "-lead.yml",
		"a\x00.yml", ".hidden.yml",
	}
	for _, in := range bad {
		if p, err := resolveSLOPath(dir, in); err == nil {
			t.Fatalf("input %q DITERIMA -> %q (harus ditolak)", in, p)
		}
	}
}

func TestResolveSLOPathAcceptsValid(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveSLOPath(dir, "payment-success-rate")
	if err != nil {
		t.Fatalf("slug valid ditolak: %v", err)
	}
	if filepath.Base(got) != "payment-success-rate.yml" {
		t.Fatalf("base = %q", got)
	}
	got, err = resolveSLOPath(dir, "custom-name.yaml")
	if err != nil {
		t.Fatalf("basename .yaml valid ditolak: %v", err)
	}
	if filepath.Base(got) != "custom-name.yaml" {
		t.Fatalf("base = %q", got)
	}
}

func TestResolveSLOPathSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.yml")
	if err := os.WriteFile(outside, []byte("x: 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Symlink di dalam sloDir menunjuk ke luar -> wajib ditolak.
	if err := os.Symlink(outside, filepath.Join(dir, "evil.yml")); err != nil {
		t.Skipf("symlink tak didukung: %v", err)
	}
	if p, err := resolveSLOPath(dir, "evil.yml"); err == nil {
		t.Fatalf("symlink escape DITERIMA -> %q", p)
	}
	// Symlink menunjuk ke dalam direktori -> boleh.
	inner := filepath.Join(dir, "inner.yml")
	if err := os.WriteFile(inner, []byte("x: 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inner, filepath.Join(dir, "alias.yml")); err != nil {
		t.Skipf("symlink tak didukung: %v", err)
	}
	if _, err := resolveSLOPath(dir, "alias.yml"); err != nil {
		t.Fatalf("symlink internal ditolak: %v", err)
	}
}

func TestLiveSLOSaveTraversalNoEscape(t *testing.T) {
	dir := t.TempDir()
	sloDir := filepath.Join(dir, "slo")
	if err := os.MkdirAll(sloDir, 0o755); err != nil {
		t.Fatal(err)
	}
	l := &liveSLO{sloDir: sloDir, workspace: dir}
	if _, err := l.Save(context.Background(), "../../../tmp/evil-u3", "x"); err == nil {
		t.Fatal("save traversal DITERIMA")
	}
	if _, err := l.Save(context.Background(), "/tmp/evil-u3.yml", "x"); err == nil {
		t.Fatal("save absolut DITERIMA")
	}
	info, err := l.Save(context.Background(), "payment-success-rate", "apiVersion: payslo/v1\n")
	if err != nil {
		t.Fatalf("save valid gagal: %v", err)
	}
	if info.File != "payment-success-rate.yml" {
		t.Fatalf("file = %q", info.File)
	}
	if _, err := os.Stat(filepath.Join(sloDir, "payment-success-rate.yml")); err != nil {
		t.Fatalf("berkas tak tertulis di sloDir: %v", err)
	}
	// Batas ukuran.
	big := strings.Repeat("x", maxSLOBytes+1)
	if _, err := l.Save(context.Background(), "payment-success-rate", big); err == nil {
		t.Fatal("konten raksasa DITERIMA")
	}
}

// ---------- durasi & budget (acceptance: 99.9%/30d ≈ 43,2 mnt) ----------

func TestParseDurationSecs(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"5s", 5, true}, {"1m", 60, true}, {"15m", 900, true},
		{"1h", 3600, true}, {"30d", 2592000, true}, {"2w", 1209600, true},
		{"90d", 7776000, true},
		{"0m", 0, false}, {"m", 0, false}, {"10x", 0, false},
		{"", 0, false}, {"1.5h", 0, false}, {"-1m", 0, false},
	}
	for _, c := range cases {
		got, ok := parseDurationSecs(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("parseDurationSecs(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestErrorBudgetAcceptance(t *testing.T) {
	month := int64(30 * 86400)
	// 99.9%/30 hari ≈ 43,2 menit (2592 detik).
	if got := errorBudgetSecs(0.999, month) / 60; math.Abs(got-43.2) > 1e-9 {
		t.Fatalf("99.9%%/30d = %v menit, want 43.2", got)
	}
	// 99.99%/30 hari ≈ 4,32 menit (259.2 detik).
	if got := errorBudgetSecs(0.9999, month) / 60; math.Abs(got-4.32) > 1e-9 {
		t.Fatalf("99.99%%/30d = %v menit, want 4.32", got)
	}
	// 99.5%/1h = 18 detik.
	if got := errorBudgetSecs(0.995, 3600); math.Abs(got-18) > 1e-9 {
		t.Fatalf("99.5%%/1h = %v dtk, want 18", got)
	}
}

// ---------- validasi berlokasi ----------

const validSLOText = `apiVersion: payslo/v1
kind: SLO
name: payment-success-rate
service: gateway-api
description: Share of payments reaching final SUCCESS.
objective: 0.995
window: 1h
sli:
  type: success-ratio
  source: metric
  good: payment_transactions_total{status="SUCCESS"}
  total: payment_transactions_total
alerting:
  runbook: runbooks/payment-success.md
  burnRates:
    - longWindow: 5m
      shortWindow: 1m
      burnRate: 14.4
      severity: page
targets:
  - backend: prometheus
`

func TestValidateSLOContentOK(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runbooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "runbooks", "payment-success.md"), []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if issues := validateSLOContent(validSLOText, dir); len(issues) != 0 {
		t.Fatalf("SLO valid dilaporkan rusak: %+v", issues)
	}
}

func TestValidateSLOContentLocated(t *testing.T) {
	// objective di baris 6 (1-based) pada teks di bawah.
	bad := strings.Replace(validSLOText, "objective: 0.995", "objective: 1.5", 1)
	issues := validateSLOContent(bad, "")
	if len(issues) == 0 {
		t.Fatal("objective 1.5 lolos")
	}
	if issues[0].Field != "objective" {
		t.Fatalf("field = %q, want objective", issues[0].Field)
	}
	if issues[0].Line != 6 {
		t.Fatalf("line = %d, want 6", issues[0].Line)
	}
	if issues[0].Column < 1 {
		t.Fatalf("column = %d", issues[0].Column)
	}
	// short >= long.
	bad2 := strings.Replace(validSLOText, "shortWindow: 1m", "shortWindow: 5m", 1)
	issues = validateSLOContent(bad2, "")
	found := false
	for _, is := range issues {
		if is.Field == "alerting.burnRates.shortWindow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("short>=long tak terdeteksi: %+v", issues)
	}
	// Field tak dikenal + kind hilang.
	bad3 := "apiVersion: payslo/v1\nname: x\nbogusField: 1\n"
	issues = validateSLOContent(bad3, "")
	fields := map[string]bool{}
	for _, is := range issues {
		fields[is.Field] = true
	}
	if !fields["bogusField"] || !fields["kind"] {
		t.Fatalf("isu tak lengkap: %+v", issues)
	}
	// YAML sintaks rusak -> baris terisi.
	issues = validateSLOContent("apiVersion: payslo/v1\n  : : :\n", "")
	if len(issues) == 0 || issues[0].Line < 1 {
		t.Fatalf("sintaks rusak tak berlokasi: %+v", issues)
	}
}

// ---------- parser output CLI ----------

func TestParsePlanApplyDriftCapabilities(t *testing.T) {
	planOut := "warning [payment-success-rate:datadog]: timeframe mapped\n" +
		"would update payment-success-rate on prometheus-local\n" +
		"would update payment-success-rate on datadog-mock\n" +
		"2 change(s) planned\n"
	p := parsePlanOutput("payment-success-rate", planOut)
	if len(p.Changes) != 2 || len(p.Warnings) != 1 {
		t.Fatalf("plan: %+v", p)
	}
	// SLO lain diabaikan (plan satu berkas).
	other := parsePlanOutput("payment-success-rate", "would update other-slo on prometheus-local\n1 change(s) planned\n")
	if len(other.Changes) != 0 {
		t.Fatalf("plan bocor SLO lain: %+v", other)
	}
	applyOut := "applied payment-success-rate on prometheus-local\n" +
		"up-to-date payment-success-rate on datadog-mock (no changes)\n"
	a := parseApplyOutput("payment-success-rate", applyOut)
	if len(a.Results) != 2 {
		t.Fatalf("apply: %+v", a)
	}
	if !a.Results[0].Applied || a.Results[0].NoOp {
		t.Fatalf("item 0 harus applied: %+v", a.Results[0])
	}
	if a.Results[1].Applied || !a.Results[1].NoOp {
		t.Fatalf("item 1 harus no-op: %+v", a.Results[1])
	}
	driftOut := "drift: payment-success-rate on datadog-mock (backend differs from last apply)\n1 drifted record(s)\n"
	dr := parseDriftOutput(driftOut)
	if dr.Clean || len(dr.Drifted) != 1 || dr.Drifted[0].Reason != "backend differs from last apply" {
		t.Fatalf("drift: %+v", dr)
	}
	if clean := parseDriftOutput("no drift\n"); !clean.Clean {
		t.Fatalf("no drift harus clean: %+v", clean)
	}
	caps := parseCapabilities("backend capabilities (see payslo/CAPABILITIES.md):\n| feature | prometheus | datadog |\n|---|---|---|\n| recording rules | yes (PromQL) | no |\n")
	if len(caps.Features) != 1 || caps.Features[0].Feature != "recording rules" {
		t.Fatalf("capabilities: %+v", caps)
	}
}

func TestUnifiedDiff(t *testing.T) {
	d := unifiedDiff("a\nb\nc\n", "a\nB\nc\n", "f.yml")
	if !strings.Contains(d, "-b\n") || !strings.Contains(d, "+B\n") {
		t.Fatalf("diff tanpa -/+:\n%s", d)
	}
	same := unifiedDiff("a\n", "a\n", "f.yml")
	for _, l := range strings.Split(same, "\n") {
		if (strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---")) ||
			(strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++")) {
			t.Fatalf("diff identik memuat perubahan %q:\n%s", l, same)
		}
	}
}

// ---------- alur demo: save -> validate gagal -> perbaiki -> plan -> apply -> no-op ----------

func TestDemoSLOFlow(t *testing.T) {
	ctx := context.Background()
	d := newDemoSLO()
	// Plan awal: latency pending (fixture), success kosong.
	p0, err := d.Plan(ctx, "payment-success-rate")
	if err != nil {
		t.Fatal(err)
	}
	if len(p0.Changes) != 0 {
		t.Fatalf("success awal harus kosong: %+v", p0.Changes)
	}
	plat, err := d.Plan(ctx, "payment-latency-p99")
	if err != nil {
		t.Fatal(err)
	}
	if len(plat.Changes) != 2 || plat.Files[0].Diff == "" {
		t.Fatalf("latency harus ber-diff prometheus: %+v", plat.Files)
	}
	// Rusakkan success -> validate gagal berlokasi.
	bad := strings.Replace(demoSLOSuccess, "objective: 0.995", "objective: 99", 1)
	if _, err := d.Save(ctx, "payment-success-rate", bad); err != nil {
		t.Fatal(err)
	}
	v, err := d.Validate(ctx, "payment-success-rate", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if v.Valid || len(v.Errors) == 0 || v.Errors[0].Field != "objective" || v.Errors[0].Line < 1 {
		t.Fatalf("validate harus gagal berlokasi: %+v", v)
	}
	// Plan menolak konten rusak (400 di handler -> inputError di sini).
	if _, err := d.Plan(ctx, "payment-success-rate"); err == nil || !isSLOInputError(err) {
		t.Fatalf("plan konten rusak harus inputError, got %v", err)
	}
	// Perbaiki (ubah burnRate agar ada diff) -> plan ber-diff -> apply.
	fixed := strings.Replace(demoSLOSuccess, "burnRate: 14.4", "burnRate: 12", 1)
	if _, err := d.Save(ctx, "payment-success-rate", fixed); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Validate(ctx, "payment-success-rate", "", false); !v.Valid {
		t.Fatalf("fixed harus valid: %+v", v.Errors)
	}
	p1, err := d.Plan(ctx, "payment-success-rate")
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Changes) != 2 {
		t.Fatalf("plan harus 2 perubahan: %+v", p1.Changes)
	}
	a1, err := d.Apply(ctx, "payment-success-rate", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range a1.Results {
		if !r.Applied || r.NoOp {
			t.Fatalf("apply pertama harus applied: %+v", r)
		}
	}
	p2, err := d.Plan(ctx, "payment-success-rate")
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Changes) != 0 {
		t.Fatalf("plan ulang harus kosong: %+v", p2.Changes)
	}
	a2, err := d.Apply(ctx, "payment-success-rate", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range a2.Results {
		if r.Applied || !r.NoOp {
			t.Fatalf("apply kedua harus no-op jelas: %+v", r)
		}
	}
	// Drift + capabilities demo.
	dr, _ := d.Drift(ctx)
	if dr.Clean || len(dr.Drifted) != 1 {
		t.Fatalf("drift demo: %+v", dr)
	}
	caps, _ := d.Capabilities(ctx)
	if len(caps.Features) != 7 {
		t.Fatalf("capabilities demo = %d baris, want 7", len(caps.Features))
	}
}

// ---------- handler HTTP (demo) ----------

func TestSLOHandlersDemo(t *testing.T) {
	s := testServer() // demo:true -> newDemoSLO
	h := s.handler()
	// List.
	rec := doJSON(t, h, "GET", "/api/v1/slos", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Detail tak dikenal -> 404.
	rec = doJSON(t, h, "GET", "/api/v1/slos/tidak-ada", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: got %d, want 404", rec.Code)
	}
	// PUT traversal -> 400 (bukan 500; tak ada file tertulis di luar).
	rec = doJSON(t, h, "PUT", "/api/v1/slos/..%2F..%2Fevil", `{"content":"x"}`, "trainee")
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Fatalf("traversal: got %d, want 400/404", rec.Code)
	}
	// Validate draf buruk -> 200 valid=false + baris.
	rec = doJSON(t, h, "POST", "/api/v1/slos/payment-success-rate/validate", `{"content":"objective: 99\n"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("validate: got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"valid":false`) || !strings.Contains(rec.Body.String(), `"line"`) {
		t.Fatalf("validate tanpa lokasi: %s", rec.Body.String())
	}
	// Apply success (masih applied) -> no-op semua.
	rec = doJSON(t, h, "POST", "/api/v1/slos/payment-success-rate/apply", `{}`, "trainee")
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"noOp":true`) {
		t.Fatalf("apply pertama demo harus no-op (sudah applied): %s", rec.Body.String())
	}
	// Drift + capabilities.
	rec = doJSON(t, h, "GET", "/api/v1/drift", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "datadog-mock") {
		t.Fatalf("drift: got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, "GET", "/api/v1/capabilities", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "recording rules") {
		t.Fatalf("capabilities: got %d (%s)", rec.Code, rec.Body.String())
	}
	// Audit mencatat save+apply.
	rec = doJSON(t, h, "GET", "/api/v1/audit?limit=50", "", "")
	if !strings.Contains(rec.Body.String(), "slo.apply") {
		t.Fatalf("audit tanpa slo.apply: %s", rec.Body.String())
	}
}
