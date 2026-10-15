package main

// SLO Center (U3): bungkus PaySLO CLI untuk lab serve.
//
// Browser TIDAK memanggil payslo/datadog-mock langsung; Go yang menjalankan
// `payslo` (subprocess, resolved via LAB_PAYSLO_BIN -> target/debug/payslo ->
// PATH) dengan CWD repo-root dan path absolut eksplisit. Output CLI (teks
// baris terstruktur) di-parse menjadi JSON untuk UI.
//
// Editor (PUT) hanya boleh menulis di <workspace>/slo/: nama dibatasi slug
// atau basename .yml/.yaml (tanpa separator -> traversal mustahil), ukuran
// dibatasi, symlink keluar direktori ditolak (resolveSLOPath).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ------------------------------------------------------------------ konstanta

const (
	maxSLOBytes   = 64 * 1024
	paysloTimeout = 90 * time.Second
)

var (
	errSLONotFound    = errors.New("SLO tidak ditemukan")
	errSLOUnavailable = errors.New("payslo tak tersedia")
	errSLOBackend     = errors.New("backend payslo gagal")
)

var (
	sloSlugRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	sloFileRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*\.(yml|yaml)$`)
	durRe      = regexp.MustCompile(`^([0-9]+)([smhdw])$`)
	yamlLineRe = regexp.MustCompile(`(?i)\bline (\d+)`)
)

// ------------------------------------------------------------------ tipe DTO

type sloSummary struct {
	Name        string   `json:"name"`
	Service     string   `json:"service"`
	Objective   float64  `json:"objective"`
	Window      string   `json:"window"`
	File        string   `json:"file"`
	Description string   `json:"description,omitempty"`
	Backends    []string `json:"backends"`
	ParseError  string   `json:"parseError,omitempty"`
}

type sliSpec struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Good   string `json:"good"`
	Total  string `json:"total"`
}

type burnRate struct {
	LongWindow  string  `json:"longWindow" yaml:"longWindow"`
	ShortWindow string  `json:"shortWindow" yaml:"shortWindow"`
	BurnRate    float64 `json:"burnRate" yaml:"burnRate"`
	Severity    string  `json:"severity" yaml:"severity"`
}

type sloTarget struct {
	Backend string `json:"backend"`
}

type sloDeployment struct {
	Connection string `json:"connection"`
	Backend    string `json:"backend"`
	Status     string `json:"status"` // applied|pending|drifted|failed
	Detail     string `json:"detail,omitempty"`
}

type sloCurrent struct {
	Status     string   `json:"status"` // ok|no_data|source_down
	ErrorRatio *float64 `json:"errorRatio,omitempty"`
	SliValue   *float64 `json:"sliValue,omitempty"`
	LastDataAt *string  `json:"lastDataAt,omitempty"`
	Detail     string   `json:"detail,omitempty"`
}

type sloBudget struct {
	WindowSecs     int64   `json:"windowSecs"`
	AllowedSecs    float64 `json:"allowedSecs"`
	AllowedMinutes float64 `json:"allowedMinutes"`
}

type sloAlert struct {
	Alert    string  `json:"alert"`
	State    string  `json:"state"`
	Severity string  `json:"severity"`
	Window   string  `json:"window,omitempty"`
	ActiveAt *string `json:"activeAt,omitempty"`
}

type sloDetail struct {
	Meta       sloSummary      `json:"meta"`
	Content    string          `json:"content"`
	Sli        sliSpec         `json:"sli"`
	BurnRates  []burnRate      `json:"burnRates"`
	Runbook    string          `json:"runbook"`
	Targets    []sloTarget     `json:"targets"`
	Deployment []sloDeployment `json:"deployment"`
	SliCurrent sloCurrent      `json:"sliCurrent"`
	Budget     sloBudget       `json:"budget"`
	Alerts     []sloAlert      `json:"alerts"`
}

type sloIssue struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type sloValidation struct {
	Valid  bool       `json:"valid"`
	Errors []sloIssue `json:"errors"`
}

type planChange struct {
	Slo        string `json:"slo"`
	Connection string `json:"connection"`
	Backend    string `json:"backend"`
}

type planFile struct {
	Connection string `json:"connection"`
	Backend    string `json:"backend"`
	Change     bool   `json:"change"`
	Desired    string `json:"desired,omitempty"`
	Actual     string `json:"actual,omitempty"`
	Diff       string `json:"diff,omitempty"`
	Note       string `json:"note,omitempty"`
}

type sloPlan struct {
	Slo      string       `json:"slo"`
	Changes  []planChange `json:"changes"`
	Warnings []string     `json:"warnings"`
	Files    []planFile   `json:"files"`
	Raw      string       `json:"raw"`
}

type applyItem struct {
	Slo        string `json:"slo"`
	Connection string `json:"connection"`
	Backend    string `json:"backend"`
	Applied    bool   `json:"applied"`
	NoOp       bool   `json:"noOp"`
	Message    string `json:"message"`
}

type sloApply struct {
	Slo      string      `json:"slo"`
	Results  []applyItem `json:"results"`
	Warnings []string    `json:"warnings"`
	Raw      string      `json:"raw"`
}

type driftItem struct {
	Slo        string `json:"slo"`
	Connection string `json:"connection"`
	Reason     string `json:"reason"`
}

type sloDrift struct {
	Drifted []driftItem `json:"drifted"`
	Clean   bool        `json:"clean"`
	Raw     string      `json:"raw"`
}

type capRow struct {
	Feature    string `json:"feature"`
	Prometheus string `json:"prometheus"`
	Datadog    string `json:"datadog"`
}

type capMatrix struct {
	Features []capRow `json:"features"`
	Raw      string   `json:"raw"`
}

type sloFileInfo struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	Bytes int    `json:"bytes"`
}

// ------------------------------------------------------------------ interface

type sloService interface {
	List(ctx context.Context) ([]sloSummary, error)
	Get(ctx context.Context, name string) (sloDetail, error)
	Save(ctx context.Context, name, content string) (sloFileInfo, error)
	Validate(ctx context.Context, name, content string, hasContent bool) (sloValidation, error)
	Plan(ctx context.Context, name string) (sloPlan, error)
	Apply(ctx context.Context, name string, connections []string) (sloApply, error)
	Drift(ctx context.Context) (sloDrift, error)
	Capabilities(ctx context.Context) (capMatrix, error)
}

// ------------------------------------------------------------------ path aman

// resolveSLOPath memetakan nama/berkas SLO ke path di dalam sloDir.
// Menolak: separator (../ dan absolut mustahil), nama tak valid, ekstensi di
// luar .yml/.yaml, dan symlink yang lolos keluar direktori.
func resolveSLOPath(sloDir, input string) (string, error) {
	if strings.ContainsRune(input, 0) {
		return "", fmt.Errorf("nama SLO mengandung NUL")
	}
	if input == "" || input == "." || input == ".." {
		return "", fmt.Errorf("nama SLO %q tidak valid", input)
	}
	if strings.ContainsAny(input, `/\`) {
		return "", fmt.Errorf("nama SLO %q ditolak: pemisah path tak diizinkan", input)
	}
	var file string
	if strings.HasSuffix(input, ".yml") || strings.HasSuffix(input, ".yaml") {
		if !sloFileRe.MatchString(input) {
			return "", fmt.Errorf("nama berkas %q tidak valid (huruf/angka/_/./- + .yml/.yaml)", input)
		}
		file = input
	} else {
		if !sloSlugRe.MatchString(input) {
			return "", fmt.Errorf("nama SLO %q tidak valid (slug a-z0-9-)", input)
		}
		file = input + ".yml"
	}
	realDir, err := filepath.EvalSymlinks(sloDir)
	if err != nil {
		return "", fmt.Errorf("direktori slo tak terbaca: %w", err)
	}
	joined := filepath.Join(realDir, file)
	if real, err := filepath.EvalSymlinks(joined); err == nil {
		joined = real
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("gagal memeriksa %q: %w", file, err)
	}
	rel, err := filepath.Rel(realDir, joined)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("nama SLO %q ditolak: lolos keluar direktori slo", input)
	}
	return joined, nil
}

// sloNameOf menormalkan input path menjadi slug (untuk route {name}).
func sloNameOf(input, resolvedPath string) string {
	base := strings.TrimSuffix(filepath.Base(resolvedPath), filepath.Ext(resolvedPath))
	if sloSlugRe.MatchString(input) {
		return input
	}
	return base
}

// ------------------------------------------------------------------ durasi & budget

// parseDurationSecs: `<int><s|m|h|d|w>` seperti payslo-model (nol ditolak).
func parseDurationSecs(raw string) (int64, bool) {
	m := durRe.FindStringSubmatch(raw)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	var unit uint64
	switch m[2] {
	case "s":
		unit = 1
	case "m":
		unit = 60
	case "h":
		unit = 3600
	case "d":
		unit = 86400
	case "w":
		unit = 604800
	}
	if n > ^uint64(0)/unit {
		return 0, false
	}
	return int64(n * unit), true
}

// errorBudgetSecs: jatah error = (1 - objective) * window. Float untuk
// tampilan (bukan uang; U6 tak berlaku di sini).
func errorBudgetSecs(objective float64, windowSecs int64) float64 {
	return (1 - objective) * float64(windowSecs)
}

func promSlug(name string) string { return strings.ReplaceAll(name, "-", "_") }

// ------------------------------------------------------------------ validasi Go

type sloDocYAML struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Name       string   `yaml:"name"`
	Service    string   `yaml:"service"`
	Objective  *float64 `yaml:"objective"`
	Window     string   `yaml:"window"`
	Sli        struct {
		Type   string `yaml:"type"`
		Source string `yaml:"source"`
		Good   string `yaml:"good"`
		Total  string `yaml:"total"`
	} `yaml:"sli"`
	Alerting struct {
		Runbook   string     `yaml:"runbook"`
		BurnRates []burnRate `yaml:"burnRates"`
	} `yaml:"alerting"`
	Targets []struct {
		Backend string `yaml:"backend"`
	} `yaml:"targets"`
}

// navSeg: langkah navigasi node YAML (kunci mapping atau indeks sekuens).
type navSeg struct {
	key   string
	idx   int
	isIdx bool
}

func segKey(k string) navSeg { return navSeg{key: k} }
func segIdx(i int) navSeg    { return navSeg{idx: i, isIdx: true} }

func yamlNavPos(root *yaml.Node, segs ...navSeg) (line, col int, ok bool) {
	node := root
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	line, col = node.Line, node.Column
	for _, s := range segs {
		if s.isIdx {
			if node == nil || node.Kind != yaml.SequenceNode || s.idx < 0 || s.idx >= len(node.Content) {
				return 0, 0, false
			}
			node = node.Content[s.idx]
			line, col = node.Line, node.Column
			continue
		}
		if node == nil || node.Kind != yaml.MappingNode {
			return 0, 0, false
		}
		found := false
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == s.key {
				line, col = node.Content[i].Line, node.Content[i].Column
				node = node.Content[i+1]
				found = true
				break
			}
		}
		if !found {
			return 0, 0, false
		}
	}
	return line, col, true
}

func yamlKeyPos(root *yaml.Node, keys ...string) (line, col int, ok bool) {
	segs := make([]navSeg, 0, len(keys))
	for _, k := range keys {
		segs = append(segs, segKey(k))
	}
	return yamlNavPos(root, segs...)
}

// validateSLOContent: pemeriksaan Go dengan lokasi (baris/kolom dari node
// YAML). workspaceDir kosong = lewati cek keberadaan runbook (mode demo).
// Dibatasi 20 isu agar tidak membanjiri UI.
func validateSLOContent(content, workspaceDir string) []sloIssue {
	var out []sloIssue
	issue := func(line, col int, field, msg string) {
		if len(out) >= 20 {
			return
		}
		if line < 1 {
			line = 1
		}
		if col < 1 {
			col = 1
		}
		out = append(out, sloIssue{Line: line, Column: col, Field: field, Message: msg})
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		line := 1
		if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
			if n, conv := strconv.Atoi(m[1]); conv == nil {
				line = n
			}
		}
		issue(line, 1, "", "YAML tidak valid: "+firstLine(err.Error()))
		return out
	}
	root := &doc
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			issue(1, 1, "", "dokumen YAML kosong")
			return out
		}
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		issue(root.Line, root.Column, "", "dokumen SLO harus mapping (bukan list/skalar)")
		return out
	}
	at := func(keys ...string) (int, int) {
		if l, c, ok := yamlKeyPos(&doc, keys...); ok {
			return l, c
		}
		return 1, 1
	}
	present := map[string]bool{}
	for i := 0; i+1 < len(root.Content); i += 2 {
		present[root.Content[i].Value] = true
	}
	allowedTop := map[string]bool{
		"apiVersion": true, "kind": true, "name": true, "service": true,
		"description": true, "objective": true, "window": true,
		"sli": true, "alerting": true, "targets": true,
	}
	for k := range present {
		if !allowedTop[k] {
			l, c := at(k)
			issue(l, c, k, "field tak dikenal (schema menolak additionalProperties)")
		}
	}
	for _, k := range []string{"apiVersion", "kind", "name", "service", "objective", "window", "sli", "alerting", "targets"} {
		if !present[k] {
			issue(1, 1, k, "field wajib "+k+" hilang")
		}
	}
	var typed sloDocYAML
	if err := root.Decode(&typed); err != nil {
		issue(1, 1, "", "bentuk YAML tak terbaca: "+firstLine(err.Error()))
		return out
	}
	if present["apiVersion"] && typed.APIVersion != "payslo/v1" {
		l, c := at("apiVersion")
		issue(l, c, "apiVersion", `harus "payslo/v1"`)
	}
	if present["kind"] && typed.Kind != "SLO" {
		l, c := at("kind")
		issue(l, c, "kind", `harus "SLO"`)
	}
	if present["name"] && !sloSlugRe.MatchString(typed.Name) {
		l, c := at("name")
		issue(l, c, "name", "harus slug a-z0-9- (mis. payment-success-rate)")
	}
	if present["objective"] && (typed.Objective == nil || !(*typed.Objective > 0 && *typed.Objective < 1)) {
		l, c := at("objective")
		issue(l, c, "objective", "harus angka di (0, 1), mis. 0.995")
	}
	windowSecs, windowOK := parseDurationSecs(typed.Window)
	if present["window"] && !windowOK {
		l, c := at("window")
		issue(l, c, "window", "durasi buruk; format <int><s|m|h|d|w>, mis. 1h atau 30d")
	}
	if present["sli"] {
		if typed.Sli.Type != "success-ratio" && typed.Sli.Type != "latency" {
			l, c := at("sli", "type")
			issue(l, c, "sli.type", "harus success-ratio atau latency")
		}
		if typed.Sli.Source != "metric" {
			l, c := at("sli", "source")
			issue(l, c, "sli.source", `harus "metric"`)
		}
		if strings.TrimSpace(typed.Sli.Good) == "" {
			l, c := at("sli", "good")
			issue(l, c, "sli.good", "query good wajib diisi")
		}
		if strings.TrimSpace(typed.Sli.Total) == "" {
			l, c := at("sli", "total")
			issue(l, c, "sli.total", "query total wajib diisi")
		}
	}
	if present["alerting"] {
		if len(typed.Alerting.BurnRates) == 0 {
			l, c := at("alerting", "burnRates")
			issue(l, c, "alerting.burnRates", "minimal satu burn rate")
		}
		for i, b := range typed.Alerting.BurnRates {
			here := func(field string) (int, int) {
				if l, c, ok := yamlNavPos(&doc, segKey("alerting"), segKey("burnRates"), segIdx(i), segKey(field)); ok {
					return l, c
				}
				return at("alerting", "burnRates")
			}
			long, okL := parseDurationSecs(b.LongWindow)
			short, okS := parseDurationSecs(b.ShortWindow)
			if !okL {
				l, c := here("longWindow")
				issue(l, c, "alerting.burnRates.longWindow", "durasi buruk untuk longWindow")
			}
			if !okS {
				l, c := here("shortWindow")
				issue(l, c, "alerting.burnRates.shortWindow", "durasi buruk untuk shortWindow")
			}
			if okL && okS && short >= long {
				l, c := here("shortWindow")
				issue(l, c, "alerting.burnRates.shortWindow", "shortWindow harus < longWindow")
			}
			if okL && windowOK && long > windowSecs {
				l, c := here("longWindow")
				issue(l, c, "alerting.burnRates.longWindow", "longWindow harus <= window objektif")
			}
			if !(b.BurnRate > 0) {
				l, c := here("burnRate")
				issue(l, c, "alerting.burnRates.burnRate", "burnRate harus > 0")
			}
			if b.Severity != "page" && b.Severity != "ticket" {
				l, c := here("severity")
				issue(l, c, "alerting.burnRates.severity", "severity harus page atau ticket")
			}
		}
		if strings.TrimSpace(typed.Alerting.Runbook) == "" {
			l, c := at("alerting", "runbook")
			issue(l, c, "alerting.runbook", "runbook wajib diisi")
		} else if workspaceDir != "" {
			clean := filepath.Clean(filepath.Join(workspaceDir, typed.Alerting.Runbook))
			if rel, err := filepath.Rel(workspaceDir, clean); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				l, c := at("alerting", "runbook")
				issue(l, c, "alerting.runbook", "runbook harus di dalam workspace")
			} else if st, err := os.Stat(clean); err != nil || st.IsDir() {
				l, c := at("alerting", "runbook")
				issue(l, c, "alerting.runbook", "runbook tidak ditemukan di workspace: "+typed.Alerting.Runbook)
			}
		}
	}
	if present["targets"] {
		if len(typed.Targets) == 0 {
			l, c := at("targets")
			issue(l, c, "targets", "minimal satu backend target")
		}
		for _, t := range typed.Targets {
			if t.Backend != "prometheus" && t.Backend != "datadog" {
				l, c := at("targets")
				issue(l, c, "targets.backend", "backend harus prometheus atau datadog")
				break
			}
			if t.Backend == "prometheus" && windowOK && (windowSecs < 60 || windowSecs > 90*86400) {
				l, c := at("window")
				issue(l, c, "window", "window di luar rentang prometheus 1m..90d")
			}
		}
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// ------------------------------------------------------------------ diff

// unifiedDiff: diff baris sederhana (LCS) actual -> desired. File SLO kecil
// (<64KB) sehingga DP O(n*m) dapat diterima; di atas 2000 baris -> ringkas.
func unifiedDiff(actual, desired, label string) string {
	a := strings.Split(actual, "\n")
	b := strings.Split(desired, "\n")
	if len(a) > 2000 || len(b) > 2000 {
		return "diff terlalu besar untuk ditampilkan (" + strconv.Itoa(len(a)) + " vs " + strconv.Itoa(len(b)) + " baris)"
	}
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var sb strings.Builder
	sb.WriteString("--- actual/" + label + "\n")
	sb.WriteString("+++ desired/" + label + "\n")
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			sb.WriteString(" " + a[i] + "\n")
			i++
			j++
		case j < m && (i >= n || dp[i][j+1] >= dp[i+1][j]):
			sb.WriteString("+" + b[j] + "\n")
			j++
		default:
			sb.WriteString("-" + a[i] + "\n")
			i++
		}
	}
	return sb.String()
}

// ------------------------------------------------------------------ live impl

type liveSLO struct {
	bin         string
	repoRoot    string
	workspace   string
	sloDir      string
	connections string
	state       string
	promBase    string
	http        *http.Client
}

type labConnection struct {
	Name     string `yaml:"name"`
	Backend  string `yaml:"backend"`
	OutDir   string `yaml:"out_dir"`
	Endpoint string `yaml:"endpoint"`
}

// newLiveSLO meresolusi path workspace dari cfg. repoRoot="." (make serve)
// -> target/debug/payslo dipakai bila ada; kalau tidak, "payslo" di PATH.
// LAB_PAYSLO_BIN selalu menang (eksplisit).
func newLiveSLO(cfg serverConfig) *liveSLO {
	workspace := cfg.workspace
	if workspace == "" {
		workspace = envOr("LAB_WORKSPACE_DIR", filepath.Join(cfg.repoRoot, "payslo-workspace"))
	}
	sloDir := cfg.sloDir
	if sloDir == "" {
		sloDir = envOr("LAB_SLO_DIR", filepath.Join(workspace, "slo"))
	}
	connections := cfg.connections
	if connections == "" {
		connections = envOr("LAB_CONNECTIONS_PATH", filepath.Join(workspace, "connections.yml"))
	}
	state := cfg.statePath
	if state == "" {
		state = envOr("LAB_STATE_PATH", filepath.Join(workspace, "state.db"))
	}
	bin := cfg.paysloBin
	if bin == "" {
		bin = envOr("LAB_PAYSLO_BIN", "")
	}
	if bin == "" {
		if cand := filepath.Join(cfg.repoRoot, "target", "debug", "payslo"); fileExists(cand) {
			if abs, err := filepath.Abs(cand); err == nil {
				bin = abs
			} else {
				bin = cand
			}
		} else {
			bin = "payslo"
		}
	}
	return &liveSLO{
		bin: bin, repoRoot: cfg.repoRoot, workspace: workspace,
		sloDir: sloDir, connections: connections, state: state,
		promBase: promBaseURL(), http: &http.Client{Timeout: promTimeout},
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func loadLabConnections(path string) ([]labConnection, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Connections []labConnection `yaml:"connections"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Connections, nil
}

func (l *liveSLO) run(ctx context.Context, args ...string) (string, error) {
	to, cancel := context.WithTimeout(ctx, paysloTimeout)
	defer cancel()
	cmd := exec.CommandContext(to, l.bin, args...)
	cmd.Dir = l.repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("%w: %q tak ada; bangun dulu (cargo build -p payslo-cli) atau set LAB_PAYSLO_BIN", errSLOUnavailable, l.bin)
		}
		if strings.Contains(msg, "DD_API_KEY") {
			msg += " (set DD_API_KEY=lab-only-dd-api-key di host)"
		}
		return stdout.String(), fmt.Errorf("%w: payslo %s: %s", errSLOBackend, strings.Join(args, " "), firstLine(msg))
	}
	return stdout.String(), nil
}

func (l *liveSLO) sloFile(name string) (string, error) {
	return l.findSLOFile(name)
}

// findSLOFile: basename .yml/.yaml langsung, atau slug -> <slug>.yml bila
// ada, bila tidak pindai direktori dan cocokkan field `name:` (nama SLO
// boleh berbeda dari nama berkas, mis. success-rate.yml berisi
// payment-success-rate).
func (l *liveSLO) findSLOFile(input string) (string, error) {
	if strings.HasSuffix(input, ".yml") || strings.HasSuffix(input, ".yaml") {
		return resolveSLOPath(l.sloDir, input)
	}
	if !sloSlugRe.MatchString(input) {
		return "", fmt.Errorf("nama SLO %q tidak valid (slug a-z0-9-)", input)
	}
	if p, err := resolveSLOPath(l.sloDir, input); err == nil && fileExists(p) {
		return p, nil
	}
	entries, err := os.ReadDir(l.sloDir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		nm := e.Name()
		if !(strings.HasSuffix(nm, ".yml") || strings.HasSuffix(nm, ".yaml")) {
			continue
		}
		full, err := resolveSLOPath(l.sloDir, nm)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var probe struct {
			Name string `yaml:"name"`
		}
		if yaml.Unmarshal(raw, &probe) != nil {
			continue
		}
		if probe.Name == input {
			return full, nil
		}
	}
	return "", errSLONotFound
}

func (l *liveSLO) readDoc(name string) (path string, doc sloDocYAML, content string, err error) {
	path, err = l.sloFile(name)
	if err != nil {
		return "", sloDocYAML{}, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", sloDocYAML{}, "", errSLONotFound
		}
		return "", sloDocYAML{}, "", err
	}
	content = string(raw)
	if yamlErr := yaml.Unmarshal(raw, &doc); yamlErr != nil {
		return path, sloDocYAML{}, content, nil // konten ada tapi tak terparse; panggil validasi
	}
	return path, doc, content, nil
}

func docToSummary(name, file string, doc sloDocYAML, content string) sloSummary {
	backends := []string{}
	for _, t := range doc.Targets {
		backends = append(backends, t.Backend)
	}
	sort.Strings(backends)
	s := sloSummary{
		Name: name, Service: doc.Service, Objective: 0,
		Window: doc.Window, File: filepath.Base(file), Backends: backends,
	}
	if doc.Objective != nil {
		s.Objective = *doc.Objective
	}
	var desc struct {
		Description string `yaml:"description"`
	}
	if yaml.Unmarshal([]byte(content), &desc) == nil {
		s.Description = desc.Description
	}
	return s
}

func (l *liveSLO) List(ctx context.Context) ([]sloSummary, error) {
	_ = ctx
	entries, err := os.ReadDir(l.sloDir)
	if err != nil {
		return nil, err
	}
	var out []sloSummary
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		nm := e.Name()
		if !(strings.HasSuffix(nm, ".yml") || strings.HasSuffix(nm, ".yaml")) {
			continue
		}
		full, err := resolveSLOPath(l.sloDir, nm)
		if err != nil {
			continue // nama aneh di disk: lewati (jujur: tak ditampilkan)
		}
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var doc sloDocYAML
		s := sloSummary{Name: strings.TrimSuffix(nm, filepath.Ext(nm)), File: nm, Backends: []string{}}
		if yamlErr := yaml.Unmarshal(raw, &doc); yamlErr != nil {
			s.ParseError = firstLine(yamlErr.Error())
		} else if doc.Name != "" {
			s = docToSummary(doc.Name, full, doc, string(raw))
		} else {
			s.ParseError = "field name hilang"
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if out == nil {
		out = []sloSummary{}
	}
	return out, nil
}

func (l *liveSLO) Get(ctx context.Context, name string) (sloDetail, error) {
	path, doc, content, err := l.readDoc(name)
	if err != nil {
		return sloDetail{}, err
	}
	slug := sloNameOf(name, path)
	meta := docToSummary(slug, path, doc, content)
	if issues := validateSLOContent(content, l.workspace); len(issues) > 0 {
		meta.ParseError = issues[0].Message
	}
	burns := []burnRate{}
	if doc.Alerting.BurnRates != nil {
		burns = doc.Alerting.BurnRates
	}
	targets := []sloTarget{}
	for _, t := range doc.Targets {
		targets = append(targets, sloTarget{Backend: t.Backend})
	}
	d := sloDetail{
		Meta: meta, Content: content,
		Sli:        sliSpec{Type: doc.Sli.Type, Source: doc.Sli.Source, Good: doc.Sli.Good, Total: doc.Sli.Total},
		BurnRates:  burns,
		Runbook:    doc.Alerting.Runbook,
		Targets:    targets,
		Deployment: []sloDeployment{},
		Alerts:     []sloAlert{},
	}
	if ws, ok := parseDurationSecs(doc.Window); ok && doc.Objective != nil {
		allow := errorBudgetSecs(*doc.Objective, ws)
		d.Budget = sloBudget{WindowSecs: ws, AllowedSecs: allow, AllowedMinutes: allow / 60}
	}
	// Deployment: plan (per koneksi) + drift (global, filter nama ini).
	if plan, perr := l.Plan(ctx, slug); perr == nil {
		drifted := map[string]bool{}
		if drift, derr := l.Drift(ctx); derr == nil {
			for _, di := range drift.Drifted {
				if di.Slo == slug {
					drifted[di.Connection] = true
				}
			}
		}
		pending := map[string]planChange{}
		for _, c := range plan.Changes {
			pending[c.Connection] = c
		}
		conns, _ := loadLabConnections(l.connections)
		for _, c := range conns {
			st := sloDeployment{Connection: c.Name, Backend: c.Backend, Status: "applied"}
			if drifted[c.Name] {
				st.Status = "drifted"
				st.Detail = "backend berbeda dari apply terakhir (lihat drift)"
			} else if ch, ok := pending[c.Name]; ok {
				st.Status = "pending"
				st.Detail = "would update " + ch.Slo + " on " + ch.Connection
			}
			d.Deployment = append(d.Deployment, st)
		}
	} else {
		d.Deployment = []sloDeployment{{Connection: "-", Backend: "-", Status: "failed", Detail: perr.Error()}}
	}
	// SLI live + alert aktif (degradasi anggun bila Prometheus mati).
	d.SliCurrent = l.sliCurrent(ctx, slug, doc.Window)
	d.Alerts = l.sloAlerts(ctx, slug)
	return d, nil
}

func (l *liveSLO) sliCurrent(ctx context.Context, slug, window string) sloCurrent {
	_ = ctx
	if _, ok := parseDurationSecs(window); !ok || window == "" {
		return sloCurrent{Status: "no_data", Detail: "window tak valid, SLI tak dapat dihitung"}
	}
	q := fmt.Sprintf("payslo:slo_error_ratio:%s:%s", promSlug(slug), window)
	samples, err := promInstant(l.promBase, q)
	if err != nil {
		return sloCurrent{Status: "source_down", Detail: "prometheus tak terjangkau"}
	}
	mv := classifyValue(samples, time.Now())
	switch mv.Status {
	case "ok":
		er := *mv.Value
		sli := 1 - er
		return sloCurrent{Status: "ok", ErrorRatio: &er, SliValue: &sli, LastDataAt: mv.LastDataAt}
	case "stale":
		return sloCurrent{Status: "no_data", Detail: "recording rule usang (apakah rules ter-apply + Prometheus reload?)"}
	default:
		return sloCurrent{Status: "no_data", Detail: "belum ada recording rule (belum apply?)"}
	}
}

func (l *liveSLO) sloAlerts(ctx context.Context, slug string) []sloAlert {
	_ = ctx
	out := []sloAlert{}
	groups, err := promRuleGroups(l.promBase, l.http)
	if err != nil {
		return out
	}
	for _, g := range groups {
		if g.Name != "payslo-"+slug {
			continue
		}
		for _, r := range g.Rules {
			if r.Alert == "" {
				continue // recording rule, bukan alert
			}
			for _, a := range r.Alerts {
				out = append(out, sloAlert{
					Alert: r.Alert, State: a.State,
					Severity: a.Labels["severity"], Window: a.Labels["window"],
					ActiveAt: a.ActiveAt,
				})
			}
		}
	}
	return out
}

func (l *liveSLO) Save(_ context.Context, name, content string) (sloFileInfo, error) {
	path, err := l.findSLOFile(name)
	if err != nil {
		if !errors.Is(err, errSLONotFound) {
			return sloFileInfo{}, err
		}
		// SLO baru: berkas <slug>.yml (satu nama = satu berkas).
		path, err = resolveSLOPath(l.sloDir, name)
		if err != nil {
			return sloFileInfo{}, err
		}
	}
	if len(content) > maxSLOBytes {
		return sloFileInfo{}, fmt.Errorf("konten %d byte melebihi batas %d byte", len(content), maxSLOBytes)
	}
	if err := os.MkdirAll(l.sloDir, 0o755); err != nil {
		return sloFileInfo{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return sloFileInfo{}, err
	}
	return sloFileInfo{Name: sloNameOf(name, path), File: filepath.Base(path), Bytes: len(content)}, nil
}

func (l *liveSLO) validateWithCLI(ctx context.Context, path string) []sloIssue {
	out, err := l.run(ctx, "validate", path, "--workspace", l.workspace)
	_ = out
	if err == nil {
		return nil
	}
	msg := err.Error()
	if i := strings.Index(msg, "payslo "); i >= 0 {
		msg = msg[i:]
	}
	return []sloIssue{{Line: 1, Column: 1, Message: "payslo validate: " + firstLine(msg)}}
}

func (l *liveSLO) Validate(ctx context.Context, name, content string, hasContent bool) (sloValidation, error) {
	text := content
	tmp := ""
	if !hasContent {
		path, err := l.sloFile(name)
		if err != nil {
			return sloValidation{}, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return sloValidation{}, errSLONotFound
			}
			return sloValidation{}, err
		}
		text = string(raw)
	} else if len(content) > maxSLOBytes {
		return sloValidation{Valid: false, Errors: []sloIssue{{Line: 1, Column: 1, Message: fmt.Sprintf("konten %d byte melebihi batas %d", len(content), maxSLOBytes)}}}, nil
	}
	if issues := validateSLOContent(text, l.workspace); len(issues) > 0 {
		return sloValidation{Valid: false, Errors: issues}, nil
	}
	// Pemeriksaan Go lolos -> otoritas CLI (skema JSON + aturan backend).
	cliPath := ""
	if hasContent {
		f, err := os.CreateTemp("", "slo-validate-*.yml")
		if err != nil {
			return sloValidation{}, err
		}
		tmp = f.Name()
		defer os.Remove(tmp)
		if _, err := f.WriteString(text); err != nil {
			_ = f.Close()
			return sloValidation{}, err
		}
		_ = f.Close()
		cliPath = tmp
	} else {
		p, err := l.sloFile(name)
		if err != nil {
			return sloValidation{}, err
		}
		cliPath = p
	}
	if issues := l.validateWithCLI(ctx, cliPath); len(issues) > 0 {
		return sloValidation{Valid: false, Errors: issues}, nil
	}
	return sloValidation{Valid: true, Errors: []sloIssue{}}, nil
}

func (l *liveSLO) Plan(ctx context.Context, name string) (sloPlan, error) {
	path, _, _, err := l.readDoc(name)
	if err != nil {
		return sloPlan{}, err
	}
	slug := sloNameOf(name, path)
	out, err := l.run(ctx, "plan", path,
		"--workspace", l.workspace, "--connections", l.connections, "--state", l.state)
	if err != nil {
		// Bedakan input buruk (400) dari backend mati (502).
		raw, _ := os.ReadFile(path)
		if issues := validateSLOContent(string(raw), l.workspace); len(issues) > 0 {
			return sloPlan{}, planApplyInputError(issues)
		}
		return sloPlan{}, err
	}
	plan := parsePlanOutput(slug, out)
	conns, cerr := loadLabConnections(l.connections)
	if cerr != nil {
		return sloPlan{}, cerr
	}
	byConn := map[string]string{}
	for _, c := range conns {
		byConn[c.Name] = c.Backend
	}
	for i := range plan.Changes {
		plan.Changes[i].Backend = byConn[plan.Changes[i].Connection]
	}
	changeSet := map[string]bool{}
	for _, c := range plan.Changes {
		changeSet[c.Connection] = true
	}
	for _, c := range conns {
		pf := planFile{Connection: c.Name, Backend: c.Backend, Change: changeSet[c.Name]}
		if c.Backend == "prometheus" && pf.Change {
			desired, derr := l.run(ctx, "render", path, "--workspace", l.workspace, "--backend", "prometheus")
			actual := ""
			if c.OutDir != "" {
				if raw, rerr := os.ReadFile(filepath.Join(c.OutDir, slug+".yml")); rerr == nil {
					actual = string(raw)
				}
			}
			if derr == nil {
				pf.Desired = desired
				pf.Actual = actual
				if actual == "" {
					pf.Note = "belum ada berkas di backend (akan dibuat saat apply)"
				}
				pf.Diff = unifiedDiff(actual, desired, slug+".yml")
			}
		}
		if c.Backend == "datadog" && pf.Change {
			pf.Note = "diff teks tak tersedia untuk backend datadog (perubahan terdeteksi via hash kanonik); lihat drift setelah apply"
		}
		plan.Files = append(plan.Files, pf)
	}
	return plan, nil
}

type inputError struct {
	issues []sloIssue
}

func (e *inputError) Error() string {
	if len(e.issues) == 0 {
		return "input SLO tidak valid"
	}
	return "SLO tidak valid: " + e.issues[0].Message
}

func planApplyInputError(issues []sloIssue) *inputError { return &inputError{issues: issues} }

func (l *liveSLO) Apply(ctx context.Context, name string, connections []string) (sloApply, error) {
	path, _, _, err := l.readDoc(name)
	if err != nil {
		return sloApply{}, err
	}
	slug := sloNameOf(name, path)
	args := []string{"apply", path, "--workspace", l.workspace,
		"--connections", l.connections, "--state", l.state}
	for _, c := range connections {
		if strings.TrimSpace(c) == "" {
			continue
		}
		args = append(args, "--connection", c)
	}
	out, err := l.run(ctx, args...)
	if err != nil {
		raw, _ := os.ReadFile(path)
		if issues := validateSLOContent(string(raw), l.workspace); len(issues) > 0 {
			return sloApply{}, planApplyInputError(issues)
		}
		return sloApply{}, err
	}
	res := parseApplyOutput(slug, out)
	conns, _ := loadLabConnections(l.connections)
	byConn := map[string]string{}
	for _, c := range conns {
		byConn[c.Name] = c.Backend
	}
	for i := range res.Results {
		if b, ok := byConn[res.Results[i].Connection]; ok {
			res.Results[i].Backend = b
		} else {
			res.Results[i].Backend = "unknown"
		}
	}
	return res, nil
}

func (l *liveSLO) Drift(ctx context.Context) (sloDrift, error) {
	out, err := l.run(ctx, "drift",
		"--workspace", l.workspace, "--connections", l.connections, "--state", l.state)
	if err != nil {
		return sloDrift{}, err
	}
	return parseDriftOutput(out), nil
}

func (l *liveSLO) Capabilities(ctx context.Context) (capMatrix, error) {
	out, err := l.run(ctx, "capabilities")
	if err != nil {
		return capMatrix{}, err
	}
	return parseCapabilities(out), nil
}

// ------------------------------------------------------------------ parser CLI

var (
	planChangeRe = regexp.MustCompile(`^would update (\S+) on (\S+)\s*$`)
	appliedRe    = regexp.MustCompile(`^applied (\S+) on (\S+)\s*$`)
	upToDateRe   = regexp.MustCompile(`^up-to-date (\S+) on (\S+) \(no changes\)\s*$`)
	driftRe      = regexp.MustCompile(`^drift: (\S+) on (\S+) \((.*)\)\s*$`)
)

func parsePlanOutput(slo, out string) sloPlan {
	p := sloPlan{Slo: slo, Changes: []planChange{}, Warnings: []string{}, Files: []planFile{}, Raw: out}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "warning "):
			p.Warnings = append(p.Warnings, t)
		case planChangeRe.MatchString(t):
			m := planChangeRe.FindStringSubmatch(t)
			if m[1] == slo {
				p.Changes = append(p.Changes, planChange{Slo: m[1], Connection: m[2]})
			}
		}
	}
	return p
}

func parseApplyOutput(slo, out string) sloApply {
	r := sloApply{Slo: slo, Results: []applyItem{}, Warnings: []string{}, Raw: out}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "warning "):
			r.Warnings = append(r.Warnings, t)
		case appliedRe.MatchString(t):
			m := appliedRe.FindStringSubmatch(t)
			if m[1] == slo {
				r.Results = append(r.Results, applyItem{Slo: m[1], Connection: m[2], Applied: true, Message: t})
			}
		case upToDateRe.MatchString(t):
			m := upToDateRe.FindStringSubmatch(t)
			if m[1] == slo {
				r.Results = append(r.Results, applyItem{Slo: m[1], Connection: m[2], NoOp: true, Message: t})
			}
		}
	}
	return r
}

func parseDriftOutput(out string) sloDrift {
	d := sloDrift{Drifted: []driftItem{}, Raw: out}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if driftRe.MatchString(t) {
			m := driftRe.FindStringSubmatch(t)
			d.Drifted = append(d.Drifted, driftItem{Slo: m[1], Connection: m[2], Reason: m[3]})
		}
	}
	d.Clean = len(d.Drifted) == 0
	return d
}

func parseCapabilities(out string) capMatrix {
	m := capMatrix{Features: []capRow{}, Raw: out}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "|") {
			continue
		}
		parts := strings.Split(t, "|")
		cols := []string{}
		for _, p := range parts {
			if c := strings.TrimSpace(p); c != "" {
				cols = append(cols, c)
			}
		}
		if len(cols) != 3 || cols[0] == "feature" || strings.Contains(cols[0], "---") {
			continue
		}
		m.Features = append(m.Features, capRow{Feature: cols[0], Prometheus: cols[1], Datadog: cols[2]})
	}
	return m
}

// ------------------------------------------------------------------ prometheus rules API

type promRuleGroup struct {
	Name  string `json:"name"`
	Rules []struct {
		Name   string            `json:"name"`
		Alert  string            `json:"alert"`
		Labels map[string]string `json:"labels"`
		Alerts []struct {
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			State       string            `json:"state"`
			ActiveAt    *string           `json:"activeAt"`
			Value       string            `json:"value"`
		} `json:"alerts"`
	} `json:"rules"`
}

func promRuleGroups(base string, client *http.Client) ([]promRuleGroup, error) {
	if client == nil {
		client = &http.Client{Timeout: promTimeout}
	}
	resp, err := client.Get(base + "/api/v1/rules")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var qr struct {
		Status string `json:"status"`
		Data   struct {
			Groups []promRuleGroup `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &qr); err != nil {
		return nil, err
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("prometheus rules status %q", qr.Status)
	}
	return qr.Data.Groups, nil
}
