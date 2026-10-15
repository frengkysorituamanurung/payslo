package main

// Fixture SLO untuk mode --demo (deterministik, tanpa payslo/CLI/Prometheus).
// Mendukung alur E2E penuh: save -> validate gagal (berlokasi) -> perbaiki ->
// plan ber-diff -> apply -> plan ulang kosong -> apply kedua no-op.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const demoSLOSuccess = `apiVersion: payslo/v1
kind: SLO
name: payment-success-rate
service: gateway-api
description: Share of payments reaching final SUCCESS across all channels.
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
    - longWindow: 1h
      shortWindow: 10m
      burnRate: 2
      severity: ticket
targets:
  - backend: prometheus
  - backend: datadog
`

const demoSLOLatency = `apiVersion: payslo/v1
kind: SLO
name: payment-latency-p99
service: gateway-api
description: Share of gateway requests served within 500ms (Envoy upstream).
objective: 0.99
window: 1h
sli:
  type: latency
  source: metric
  good: envoy_cluster_upstream_rq_time_bucket{envoy_cluster_name="gateway-api",le="500"}
  total: envoy_cluster_upstream_rq_time_bucket{envoy_cluster_name="gateway-api",le="+Inf"}
alerting:
  runbook: runbooks/payment-latency.md
  burnRates:
    - longWindow: 15m
      shortWindow: 5m
      burnRate: 6
      severity: page
targets:
  - backend: prometheus
`

var demoConnections = []labConnection{
	{Name: "prometheus-local", Backend: "prometheus", OutDir: "payslo-workspace/generated"},
	{Name: "datadog-mock", Backend: "datadog", Endpoint: "http://127.0.0.1:8126"},
}

var demoCapRows = []capRow{
	{"recording rules", "yes (PromQL)", "no (Datadog evaluates SLI itself)"},
	{"burn-rate alerts", "yes (MWMB, any window ≥1m)", "yes (error_budget monitors, timeframe 7d/30d/90d only)"},
	{"objective windows", "1m..90d", "7d/30d/90d only (lab windows mapped to 7d + warning)"},
	{"SLI source", "metric (PromQL, native)", "metric (Datadog queries; PromQL stored opaquely, not evaluated)"},
	{"drift check", "yes (file hash)", "yes (API content hash)"},
	{"atomic apply", "yes (temp + rename + reload)", "yes (SLO first, then monitors; retry 429/5xx)"},
	{"runbook link", "yes (runbook_url annotation)", "yes (message + tag)"},
}

type demoSLO struct {
	mu      sync.Mutex
	files   map[string]string // name -> konten saat ini
	applied map[string]string // name -> konten terakhir di-apply
}

func newDemoSLO() *demoSLO {
	// latency sengaja BERBEDA dari snapshot apply (ada perubahan pending).
	latencyEdited := strings.Replace(demoSLOLatency, "burnRate: 6", "burnRate: 8", 1)
	return &demoSLO{
		files: map[string]string{
			"payment-success-rate": demoSLOSuccess,
			"payment-latency-p99":  latencyEdited,
		},
		applied: map[string]string{
			"payment-success-rate": demoSLOSuccess,
			"payment-latency-p99":  demoSLOLatency,
		},
	}
}

func (d *demoSLO) List(_ context.Context) ([]sloSummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := []sloSummary{}
	for name, content := range d.files {
		s := demoSummaryOf(name, content)
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func demoSummaryOf(name, content string) sloSummary {
	var doc sloDocYAML
	s := sloSummary{Name: name, File: name + ".yml", Backends: []string{}}
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		s.ParseError = firstLine(err.Error())
		return s
	}
	if doc.Name == "" {
		s.ParseError = "field name hilang"
		return s
	}
	backends := []string{}
	for _, t := range doc.Targets {
		backends = append(backends, t.Backend)
	}
	sort.Strings(backends)
	obj := 0.0
	if doc.Objective != nil {
		obj = *doc.Objective
	}
	return sloSummary{
		Name: doc.Name, Service: doc.Service, Objective: obj,
		Window: doc.Window, File: name + ".yml", Backends: backends,
	}
}

func (d *demoSLO) Get(_ context.Context, name string) (sloDetail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	content, ok := d.files[name]
	if !ok {
		return sloDetail{}, errSLONotFound
	}
	return d.detailLocked(name, content), nil
}

func (d *demoSLO) detailLocked(name, content string) sloDetail {
	meta := demoSummaryOf(name, content)
	var doc sloDocYAML
	_ = yaml.Unmarshal([]byte(content), &doc)
	burns := []burnRate{}
	if doc.Alerting.BurnRates != nil {
		burns = doc.Alerting.BurnRates
	}
	targets := []sloTarget{}
	for _, t := range doc.Targets {
		targets = append(targets, sloTarget{Backend: t.Backend})
	}
	deploy := []sloDeployment{}
	for _, c := range demoConnections {
		st := sloDeployment{Connection: c.Name, Backend: c.Backend, Status: "applied"}
		if d.files[name] != d.applied[name] {
			st.Status = "pending"
			st.Detail = "would update " + name + " on " + c.Name
		}
		deploy = append(deploy, st)
	}
	// Drift demo: latency/datadog-mock selalu drifted (contoh edit manual).
	for i := range deploy {
		if name == "payment-latency-p99" && deploy[i].Connection == "datadog-mock" {
			deploy[i].Status = "drifted"
			deploy[i].Detail = "backend berbeda dari apply terakhir (lihat drift)"
		}
	}
	er, sli := 0.002, 0.998
	at := "2026-10-16T07:00:00Z"
	budget := sloBudget{}
	if ws, ok := parseDurationSecs(doc.Window); ok && doc.Objective != nil {
		allow := errorBudgetSecs(*doc.Objective, ws)
		budget = sloBudget{WindowSecs: ws, AllowedSecs: allow, AllowedMinutes: allow / 60}
	}
	alerts := []sloAlert{}
	if name == "payment-latency-p99" {
		alerts = append(alerts, sloAlert{Alert: "PaySLOBurnRateHigh", State: "firing", Severity: "page", Window: "15m", ActiveAt: &at})
	}
	return sloDetail{
		Meta: meta, Content: content,
		Sli:        sliSpec{Type: doc.Sli.Type, Source: doc.Sli.Source, Good: doc.Sli.Good, Total: doc.Sli.Total},
		BurnRates:  burns,
		Runbook:    doc.Alerting.Runbook,
		Targets:    targets,
		Deployment: deploy,
		SliCurrent: sloCurrent{Status: "ok", ErrorRatio: &er, SliValue: &sli, LastDataAt: &at},
		Budget:     budget,
		Alerts:     alerts,
	}
}

func (d *demoSLO) Save(_ context.Context, name, content string) (sloFileInfo, error) {
	if !sloSlugRe.MatchString(name) {
		return sloFileInfo{}, fmt.Errorf("nama SLO %q tidak valid (slug a-z0-9-)", name)
	}
	if len(content) > maxSLOBytes {
		return sloFileInfo{}, fmt.Errorf("konten %d byte melebihi batas %d byte", len(content), maxSLOBytes)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.files[name]; !ok {
		return sloFileInfo{}, errSLONotFound
	}
	d.files[name] = content
	return sloFileInfo{Name: name, File: name + ".yml", Bytes: len(content)}, nil
}

func (d *demoSLO) Validate(_ context.Context, name, content string, hasContent bool) (sloValidation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	text := content
	if !hasContent {
		var ok bool
		text, ok = d.files[name]
		if !ok {
			return sloValidation{}, errSLONotFound
		}
	} else if len(content) > maxSLOBytes {
		return sloValidation{Valid: false, Errors: []sloIssue{{Line: 1, Column: 1, Message: "konten melebihi batas"}}}, nil
	}
	// Demo: pemeriksaan Go tanpa runbook (tanpa workspace) + pesan CLI semu
	// bila nama SLO di konten != nama berkas (aturan doc-vs-file).
	issues := validateSLOContent(text, "")
	if len(issues) > 0 {
		return sloValidation{Valid: false, Errors: issues}, nil
	}
	return sloValidation{Valid: true, Errors: []sloIssue{}}, nil
}

func (d *demoSLO) Plan(_ context.Context, name string) (sloPlan, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	content, ok := d.files[name]
	if !ok {
		return sloPlan{}, errSLONotFound
	}
	if issues := validateSLOContent(content, ""); len(issues) > 0 {
		return sloPlan{}, planApplyInputError(issues)
	}
	p := sloPlan{Slo: name, Changes: []planChange{}, Warnings: []string{}, Files: []planFile{}, Raw: ""}
	if content != d.applied[name] {
		for _, c := range demoConnections {
			p.Changes = append(p.Changes, planChange{Slo: name, Connection: c.Name, Backend: c.Backend})
			p.Raw += "would update " + name + " on " + c.Name + "\n"
		}
		if strings.Contains(content, "datadog") {
			p.Warnings = append(p.Warnings, "warning ["+name+":datadog]: datadog SLO timeframes are 7d/30d/90d only: window 1h mapped to 7d")
		}
		for _, c := range demoConnections {
			pf := planFile{Connection: c.Name, Backend: c.Backend, Change: true}
			if c.Backend == "prometheus" {
				pf.Desired = "# rendered prometheus rules untuk " + name + "\n" + content
				pf.Actual = "# rules ter-apply sebelumnya\n" + d.applied[name]
				pf.Diff = unifiedDiff(pf.Actual, pf.Desired, name+".yml")
			} else {
				pf.Note = "diff teks tak tersedia untuk backend datadog (perubahan terdeteksi via hash kanonik); lihat drift setelah apply"
			}
			p.Files = append(p.Files, pf)
		}
		p.Raw += fmt.Sprintf("%d change(s) planned", len(p.Changes))
	} else {
		for _, c := range demoConnections {
			p.Files = append(p.Files, planFile{Connection: c.Name, Backend: c.Backend})
		}
		p.Raw = "0 change(s) planned"
	}
	return p, nil
}

func (d *demoSLO) Apply(_ context.Context, name string, connections []string) (sloApply, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	content, ok := d.files[name]
	if !ok {
		return sloApply{}, errSLONotFound
	}
	if issues := validateSLOContent(content, ""); len(issues) > 0 {
		return sloApply{}, planApplyInputError(issues)
	}
	want := map[string]bool{}
	for _, c := range connections {
		want[c] = true
	}
	r := sloApply{Slo: name, Results: []applyItem{}, Warnings: []string{}, Raw: ""}
	for _, c := range demoConnections {
		if len(want) > 0 && !want[c.Name] {
			continue
		}
		if d.applied[name] == content {
			r.Results = append(r.Results, applyItem{Slo: name, Connection: c.Name, Backend: c.Backend, NoOp: true, Message: "up-to-date " + name + " on " + c.Name + " (no changes)"})
			r.Raw += "up-to-date " + name + " on " + c.Name + " (no changes)\n"
		} else {
			r.Results = append(r.Results, applyItem{Slo: name, Connection: c.Name, Backend: c.Backend, Applied: true, Message: "applied " + name + " on " + c.Name})
			r.Raw += "applied " + name + " on " + c.Name + "\n"
		}
	}
	if len(r.Results) == 0 {
		return sloApply{}, fmt.Errorf("unknown connection (pilih: prometheus-local, datadog-mock)")
	}
	d.applied[name] = content
	return r, nil
}

func (d *demoSLO) Drift(_ context.Context) (sloDrift, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	items := []driftItem{
		{Slo: "payment-latency-p99", Connection: "datadog-mock", Reason: "backend differs from last apply"},
	}
	raw := "drift: payment-latency-p99 on datadog-mock (backend differs from last apply)\n1 drifted record(s)"
	return sloDrift{Drifted: items, Clean: false, Raw: raw}, nil
}

func (d *demoSLO) Capabilities(_ context.Context) (capMatrix, error) {
	raw := "backend capabilities (see payslo/CAPABILITIES.md):\n| feature | prometheus | datadog |\n|---|---|---|\n"
	for _, r := range demoCapRows {
		raw += "| " + r.Feature + " | " + r.Prometheus + " | " + r.Datadog + " |\n"
	}
	return capMatrix{Features: demoCapRows, Raw: raw}, nil
}
