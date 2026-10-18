package main

// Runbook tereksekusi (U4): definisi YAML milik lab serve di
// payslo-workspace/runbooks/<slo>.yml. BUKAN payslo-runbook (masih skeleton).
// Aturan keras: diagnostic = promql (nama allowlist)/http GET loopback;
// remediation = HANYA lab_routing + impact wajib + requires_approval HARUS
// true (ditolak saat load bila tidak).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxRunbookBytes = 64 * 1024

// ------------------------------------------------------------------ format

type runSpec struct {
	Type     string   `yaml:"type" json:"type"`
	Query    string   `yaml:"query,omitempty" json:"query,omitempty"`
	Provider string   `yaml:"provider,omitempty" json:"provider,omitempty"`
	Method   string   `yaml:"method,omitempty" json:"method,omitempty"`
	URL      string   `yaml:"url,omitempty" json:"url,omitempty"`
	Disable  []string `yaml:"disable,omitempty" json:"disable,omitempty"`
	Enable   []string `yaml:"enable,omitempty" json:"enable,omitempty"`
}

type runbookStep struct {
	Name             string  `yaml:"name" json:"name"`
	Kind             string  `yaml:"kind" json:"kind"` // diagnostic|remediation|manual
	Action           string  `yaml:"action,omitempty" json:"action"`
	Impact           string  `yaml:"impact,omitempty" json:"impact,omitempty"`
	Detail           string  `yaml:"detail,omitempty" json:"detail,omitempty"`
	RequiresApproval *bool   `yaml:"requires_approval,omitempty" json:"-"`
	Run              runSpec `yaml:"run,omitempty" json:"run,omitempty"`
}

type runbookDoc struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Name       string        `yaml:"name"`
	Slo        string        `yaml:"slo"`
	Steps      []runbookStep `yaml:"steps"`
}

// stepView: langkah + status approval terkait (bila remediation).
// Run ikut diserialkan (definisi server-side, bukan input browser).
type stepView struct {
	Idx              int       `json:"idx"`
	Name             string    `json:"name"`
	Kind             string    `json:"kind"`
	Action           string    `json:"action"`
	Impact           string    `json:"impact,omitempty"`
	Detail           string    `json:"detail,omitempty"`
	RequiresApproval bool      `json:"requiresApproval"`
	Run              runSpec   `json:"run,omitempty"`
	Approval         *approval `json:"approval,omitempty"`
}

type runbookView struct {
	Present bool       `json:"present"`
	File    string     `json:"file,omitempty"`
	Steps   []stepView `json:"steps"`
}

type stepOutput struct {
	Status string         `json:"status"` // ok|no_data|source_down|failed
	Detail string         `json:"detail,omitempty"`
	Values map[string]any `json:"values,omitempty"`
}

// ------------------------------------------------------------------ store

type runbookStore struct {
	dir string
}

func (s *runbookStore) pathForSLO(slo string) (string, bool) {
	if !sloSlugRe.MatchString(slo) {
		return "", false
	}
	for _, ext := range []string{".yml", ".yaml"} {
		p := filepath.Join(s.dir, slo+ext)
		if fileExists(p) {
			return p, true
		}
	}
	return "", false
}

func defaultAction(step runbookStep) string {
	if step.Action != "" {
		return step.Action
	}
	if step.Run.Type == "lab_routing" && len(step.Run.Disable) > 0 {
		return "disable provider " + strings.Join(step.Run.Disable, ",")
	}
	if step.Run.Type == "lab_routing" && len(step.Run.Enable) > 0 {
		return "enable provider " + strings.Join(step.Run.Enable, ",")
	}
	return step.Name
}

func (s *runbookStore) forSLO(slo string) runbookView {
	path, ok := s.pathForSLO(slo)
	if !ok {
		return runbookView{Present: false, Steps: []stepView{}}
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > maxRunbookBytes {
		return runbookView{Present: false, Steps: []stepView{}}
	}
	var doc runbookDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return runbookView{Present: false, Steps: []stepView{}}
	}
	if loadErr := validateRunbookDoc(&doc); loadErr != "" {
		return runbookView{Present: false, Steps: []stepView{}}
	}
	steps := make([]stepView, 0, len(doc.Steps))
	for i, st := range doc.Steps {
		steps = append(steps, stepView{
			Idx: i, Name: st.Name, Kind: st.Kind, Action: defaultAction(st),
			Impact: st.Impact, Detail: st.Detail, RequiresApproval: st.Kind == "remediation",
			Run: st.Run,
		})
	}
	return runbookView{Present: true, File: filepath.Base(path), Steps: steps}
}

// validateRunbookDoc mengembalikan pesan error ("" = valid). Remediation
// tanpa requires_approval eksplisit true DITOLAK saat load.
func validateRunbookDoc(doc *runbookDoc) string {
	if doc.APIVersion != "payslo/v1" {
		return `apiVersion harus "payslo/v1"`
	}
	if doc.Kind != "Runbook" {
		return `kind harus "Runbook"`
	}
	if doc.Slo == "" || !sloSlugRe.MatchString(doc.Slo) {
		return "slo harus slug valid"
	}
	if len(doc.Steps) == 0 {
		return "steps minimal satu"
	}
	seen := map[string]bool{}
	for i := range doc.Steps {
		st := &doc.Steps[i]
		if strings.TrimSpace(st.Name) == "" {
			return fmt.Sprintf("steps[%d]: name wajib", i)
		}
		if seen[st.Name] {
			return fmt.Sprintf("steps[%d]: name duplikat %q", i, st.Name)
		}
		seen[st.Name] = true
		switch st.Kind {
		case "diagnostic":
			switch st.Run.Type {
			case "promql":
				if metricQueries[st.Run.Query].build == nil {
					return fmt.Sprintf("steps[%d]: query promql %q bukan allowlist", i, st.Run.Query)
				}
				if st.Run.Provider != "" && st.Run.Provider != "all" && !validProviders[st.Run.Provider] {
					return fmt.Sprintf("steps[%d]: provider %q tak dikenal", i, st.Run.Provider)
				}
			case "http":
				if msg := validateDiagnosticURL(st.Run.Method, st.Run.URL); msg != "" {
					return fmt.Sprintf("steps[%d]: %s", i, msg)
				}
			default:
				return fmt.Sprintf("steps[%d]: diagnostic run.type harus promql|http", i)
			}
		case "remediation":
			if st.Run.Type != "lab_routing" {
				return fmt.Sprintf("steps[%d]: remediation run.type harus lab_routing", i)
			}
			if st.RequiresApproval == nil || !*st.RequiresApproval {
				return fmt.Sprintf("steps[%d]: remediation WAJIB requires_approval: true", i)
			}
			if strings.TrimSpace(st.Impact) == "" {
				return fmt.Sprintf("steps[%d]: remediation WAJIB impact", i)
			}
			if len(st.Run.Disable)+len(st.Run.Enable) == 0 {
				return fmt.Sprintf("steps[%d]: lab_routing butuh disable/enable", i)
			}
			for _, p := range append(append([]string{}, st.Run.Disable...), st.Run.Enable...) {
				if !validProviders[p] {
					return fmt.Sprintf("steps[%d]: provider %q tak dikenal", i, p)
				}
			}
		case "manual":
			// Tak ada eksekusi; detail opsional.
		default:
			return fmt.Sprintf("steps[%d]: kind harus diagnostic|remediation|manual", i)
		}
	}
	return ""
}

func validateDiagnosticURL(method, rawURL string) string {
	if method != "" && method != "GET" {
		return "http diagnostic hanya boleh GET"
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return "url http tak valid (http://loopback...)"
	}
	host := strings.ToLower(u.Hostname())
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "http diagnostic hanya ke loopback lab"
	}
	return ""
}

func (v runbookView) stepForAction(action string) (stepView, bool) {
	for _, st := range v.Steps {
		if st.Action == action {
			return st, true
		}
	}
	return stepView{}, false
}

func (v runbookView) withApprovals(approvals []approval) runbookView {
	byAction := map[string]*approval{}
	for i := range approvals {
		a := &approvals[i]
		if cur, ok := byAction[a.Action]; !ok || a.RequestedAt > cur.RequestedAt {
			byAction[a.Action] = a
		}
	}
	for i := range v.Steps {
		if a, ok := byAction[v.Steps[i].Action]; ok {
			cpy := *a
			v.Steps[i].Approval = &cpy
		}
	}
	return v
}

// ------------------------------------------------------------------ eksekutor

func runDiagnostic(ctx context.Context, run runSpec) (stepOutput, error) {
	switch run.Type {
	case "promql":
		return runPromqlDiagnostic(run)
	case "http":
		return runHTTPDiagnostic(ctx, run)
	default:
		return stepOutput{}, fmt.Errorf("executor %q tak dikenal", run.Type)
	}
}

func runPromqlDiagnostic(run runSpec) (stepOutput, error) {
	nq, ok := metricQueries[run.Query]
	if !ok || nq.build == nil {
		return stepOutput{}, fmt.Errorf("query %q bukan allowlist", run.Query)
	}
	base := promBaseURL()
	now := time.Now()
	if run.Query == "payment_success_ratio_by_provider" || nq.needsProvider {
		providers := []string{"bca", "mandiri", "ewallet"}
		if run.Provider != "" && run.Provider != "all" {
			providers = []string{run.Provider}
		}
		values := map[string]any{}
		anyOK := false
		for _, p := range providers {
			samples, err := promInstant(base, nq.build("5m", p))
			if err != nil {
				values[p] = map[string]any{"status": "source_down"}
				continue
			}
			mv := classifyValue(samples, now)
			if mv.Status == "ok" && mv.Value != nil {
				anyOK = true
				values[p] = map[string]any{"status": "ok", "value": *mv.Value}
			} else {
				values[p] = map[string]any{"status": mv.Status}
			}
		}
		if !anyOK {
			return stepOutput{Status: "no_data", Detail: "tak ada data provider", Values: values}, nil
		}
		return stepOutput{Status: "ok", Values: values}, nil
	}
	samples, err := promInstant(base, nq.build("5m", ""))
	if err != nil {
		return stepOutput{Status: "source_down", Detail: "prometheus tak terjangkau"}, nil
	}
	mv := classifyValue(samples, now)
	if mv.Status != "ok" || mv.Value == nil {
		return stepOutput{Status: mv.Status, Detail: mv.Detail}, nil
	}
	return stepOutput{Status: "ok", Values: map[string]any{"value": *mv.Value}}, nil
}

func runHTTPDiagnostic(ctx context.Context, run runSpec) (stepOutput, error) {
	if msg := validateDiagnosticURL(run.Method, run.URL); msg != "" {
		return stepOutput{}, fmt.Errorf("%s", msg)
	}
	method := run.Method
	if method == "" {
		method = "GET"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, run.URL, nil)
	if err != nil {
		return stepOutput{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return stepOutput{Status: "source_down", Detail: "target tak terjangkau"}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	var parsed any
	out := map[string]any{"httpStatus": resp.StatusCode}
	if json.Unmarshal(body, &parsed) == nil {
		out["body"] = parsed
	} else {
		out["body"] = string(body)
	}
	return stepOutput{Status: "ok", Values: out}, nil
}

// runLabRouting mengeksekusi remediation: baca routing, terapkan
// disable/enable, PUT kembali. Idempoten: bila sudah sesuai -> no-op jujur.
func runLabRouting(ctx context.Context, ctrl control, run runSpec) (stepOutput, error) {
	current, err := ctrl.Routing(ctx)
	if err != nil {
		return stepOutput{}, fmt.Errorf("baca routing gagal: %w", err)
	}
	next := current
	setEnabled := func(name string, enabled bool) {
		switch name {
		case "bca":
			next.Bca.Enabled = enabled
		case "mandiri":
			next.Mandiri.Enabled = enabled
		case "ewallet":
			next.Ewallet.Enabled = enabled
		}
	}
	for _, p := range run.Disable {
		setEnabled(p, false)
	}
	for _, p := range run.Enable {
		setEnabled(p, true)
	}
	if next == current {
		return stepOutput{Status: "ok", Detail: "routing sudah sesuai (no-op, tanpa PUT)"}, nil
	}
	updated, err := ctrl.SetRouting(ctx, next)
	if err != nil {
		return stepOutput{Status: "failed", Detail: "PUT routing gagal: " + err.Error()}, err
	}
	raw, _ := json.Marshal(updated)
	return stepOutput{Status: "ok", Detail: "routing diterapkan", Values: map[string]any{"routing": json.RawMessage(raw)}}, nil
}
