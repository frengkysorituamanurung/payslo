package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type serverConfig struct {
	demo     bool
	devProxy string
	// webDir override disk (dev/test). Kosong = pakai embed ui/dist.
	webDir string
	token  string
	port   string
	// U2: backend kontrol + pembayaran.
	auditPath string
	repoRoot  string
	gateway   string // ingress (Envoy)
	adminURL  string
	bankURLs  map[string]string
	grafana   string
	loki      string
	tempo     string
	lokiName  string
	// U3: bungkus PaySLO (SLO-as-code) + direktori workspace.
	workspace   string
	sloDir      string
	connections string
	statePath   string
	paysloBin   string
	// U4: insiden (proksi payslo-server) + runbook/postmortem milik lab serve.
	paysloURL  string
	consoleDir string
	// U6: mode mentor + snapshot + belajar.
	mentorPIN   string
	merchantURL string
}

// webDist tertanam saat build (Makefile menyalin ui/dist ke sini via ui-build).
//
//go:embed web/dist
var webDist embed.FS

type server struct {
	cfg         serverConfig
	mux         *http.ServeMux
	routes      []string // "METHOD /path" — kontrak diuji vs api/openapi.yaml
	payments    payments
	control     control
	audit       *auditor
	slos        sloService
	incidents   incidents
	postmortems postmortemSvc
	settings    settingsSvc
	// U6: gate PIN mentor (pembatas spoiler), progres SQLite, snapshot demo.
	mentor          *mentorGate
	progress        *progressStore
	demoSnapshots   map[string]*demoSnapshotState
	demoSnapshotsMu sync.Mutex
	httpCli         *http.Client
	bankURLs        map[string]string
	merchantURL     string
	adminURL        string
}

func (s *server) httpClient() *http.Client { return s.httpCli }

// postmortemSvc/settingsSvc: live = file di lab-console/; demo = in-memory.
type postmortemSvc interface {
	Get(incident incidentSummary, id string) (postmortemDoc, error)
	Save(id, status string, sections postmortemSections, by string) (postmortemDoc, error)
}

type settingsSvc interface {
	Get() labSettings
	Set(name string) (labSettings, error)
}

type ctxKey string

const nonceKey ctxKey = "csp-nonce"

func newServer(cfg serverConfig) *server {
	if cfg.token == "" {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		cfg.token = hex.EncodeToString(b)
	}
	if cfg.lokiName == "" {
		cfg.lokiName = "LOKI"
	}
	if cfg.grafana == "" {
		cfg.grafana = envOr("LAB_GRAFANA_URL", "http://127.0.0.1:3002")
	}
	if cfg.loki == "" {
		cfg.loki = envOr("LAB_LOKI_URL", "")
	}
	if cfg.tempo == "" {
		cfg.tempo = envOr("LAB_TEMPO_URL", "")
	}
	if cfg.gateway == "" {
		cfg.gateway = envOr("LAB_GATEWAY_INGRESS_URL", "http://127.0.0.1:8080")
	}
	if cfg.adminURL == "" {
		cfg.adminURL = envOr("LAB_GATEWAY_ADMIN_URL", "http://127.0.0.1:3001")
	}
	if cfg.repoRoot == "" {
		cfg.repoRoot = envOr("LAB_REPO_ROOT", ".")
	}
	if cfg.adminURL == "" {
		cfg.adminURL = envOr("LAB_GATEWAY_ADMIN_URL", "http://127.0.0.1:3001")
	}
	if cfg.merchantURL == "" {
		cfg.merchantURL = envOr("LAB_MERCHANT_URL", "http://127.0.0.1:5000")
	}
	if cfg.auditPath == "" && !cfg.demo {
		cfg.auditPath = envOr("LAB_AUDIT_PATH", defaultAuditPath(cfg.repoRoot))
	}
	bankURLs := cfg.bankURLs
	if bankURLs == nil {
		bankURLs = map[string]string{
			"bca":     envOr("LAB_BANK_BCA_URL", "http://127.0.0.1:4001"),
			"mandiri": envOr("LAB_BANK_MANDIRI_URL", "http://127.0.0.1:4002"),
			"ewallet": envOr("LAB_BANK_EWALLET_URL", "http://127.0.0.1:4003"),
		}
	}
	s := &server{
		cfg: cfg, mux: http.NewServeMux(), audit: newAuditor(cfg.auditPath),
		bankURLs: bankURLs,
		httpCli:  &http.Client{Timeout: 10 * time.Second},
	}
	s.mentor = newMentorGate(cfg.mentorPIN)
	workspace := cfg.workspace
	if workspace == "" {
		workspace = envOr("LAB_WORKSPACE_DIR", filepath.Join(cfg.repoRoot, "payslo-workspace"))
	}
	consoleDir := cfg.consoleDir
	if consoleDir == "" {
		consoleDir = envOr("LAB_CONSOLE_DIR", filepath.Join(workspace, "lab-console"))
	}
	s.postmortems = &postmortemStore{dir: filepath.Join(consoleDir, "postmortems")}
	s.settings = &settingsStore{path: filepath.Join(consoleDir, "settings.json")}
	s.demoSnapshots = map[string]*demoSnapshotState{}
	progress, err := openProgressStore(consoleDir)
	if err != nil {
		// Progres belajar gagal terbuka -> server tetap jalan, endpoint learn
		// akan menjawab 502/500 dengan penyebab jelas (jujur, tak diamkan).
		progress = nil
	}
	s.progress = progress
	if cfg.demo {
		s.payments = newDemoPayments()
		s.control = newDemoControl()
		s.slos = newDemoSLO()
		demo := newDemoIncidents()
		s.incidents = demo
		s.postmortems = &demoPMSvc{d: demo}
		s.settings = &demoSettingsSvc{d: demo}
	} else {
		s.payments = newPgPayments(bankEndpoints{
			urls: bankURLs, grafana: cfg.grafana, loki: cfg.loki, tempo: cfg.tempo,
			lokiName: cfg.lokiName, ingress: cfg.gateway, adminURL: cfg.adminURL,
		})
		s.control = newLiveControl(cfg.adminURL, cfg.repoRoot)
		s.slos = newLiveSLO(cfg)
		s.incidents = newLiveIncidents(cfg, workspace, s.control, s.audit, s.postmortems, s.settings)
	}
	s.registerRoutes()
	return s
}

// demoPMSvc/demoSettingsSvc: adaptor demoIncidents ke interface file-store.
type demoPMSvc struct{ d *demoIncidents }

func (p *demoPMSvc) Get(incident incidentSummary, id string) (postmortemDoc, error) {
	return p.d.GetPM(incident, id)
}

func (p *demoPMSvc) Save(id, status string, sections postmortemSections, by string) (postmortemDoc, error) {
	return p.d.SavePM(id, status, sections, by)
}

type demoSettingsSvc struct{ d *demoIncidents }

func (p *demoSettingsSvc) Get() labSettings { return p.d.GetSettings() }

func (p *demoSettingsSvc) Set(name string) (labSettings, error) {
	return p.d.SetSettings(name)
}

// registerRoutes memetakan route ke handler dan mencatatnya di s.routes agar
// test kontrak bisa membandingkan dengan api/openapi.yaml.
func (s *server) registerRoutes() {
	route := func(pattern string, h http.HandlerFunc) {
		s.routes = append(s.routes, pattern)
		s.mux.HandleFunc(pattern, h)
	}
	route("GET /api/v1/session", s.handleSession)
	route("GET /api/v1/health/stack", s.handleStack)
	route("GET /api/v1/metrics/summary", s.handleSummary)
	route("GET /api/v1/metrics/series", s.handleSeries)
	route("GET /api/v1/events", s.handleEvents)
	// U2.
	route("GET /api/v1/payments", s.handlePaymentsList)
	route("POST /api/v1/payments", s.handlePaymentCreate)
	route("GET /api/v1/payments/{id}", s.handlePaymentDetail)
	route("GET /api/v1/payments/{id}/bank-status", s.handleBankStatus)
	route("GET /api/v1/traffic", s.handleTrafficGet)
	route("PUT /api/v1/traffic", s.handleTrafficPut)
	route("GET /api/v1/routing", s.handleRoutingGet)
	route("PUT /api/v1/routing", s.handleRoutingPut)
	route("PUT /api/v1/release", s.handleReleasePut)
	route("GET /api/v1/audit", s.handleAuditList)
	// U3.
	route("GET /api/v1/slos", s.handleSLOList)
	route("GET /api/v1/slos/{name}", s.handleSLOGet)
	route("PUT /api/v1/slos/{name}", s.handleSLOSave)
	route("POST /api/v1/slos/{name}/validate", s.handleSLOValidate)
	route("POST /api/v1/slos/{name}/plan", s.handleSLOPlan)
	route("POST /api/v1/slos/{name}/apply", s.handleSLOApply)
	route("GET /api/v1/drift", s.handleDrift)
	route("GET /api/v1/capabilities", s.handleCapabilities)
	// U4.
	route("GET /api/v1/incidents", s.handleIncidentList)
	route("GET /api/v1/incidents/{id}", s.handleIncidentGet)
	route("POST /api/v1/incidents/{id}/notes", s.handleIncidentNote)
	route("GET /api/v1/incidents/{id}/approvals", s.handleIncidentApprovals)
	route("POST /api/v1/incidents/{id}/approvals", s.handleApprovalRequest)
	route("POST /api/v1/incidents/{id}/runbook/steps/{idx}/run", s.handleRunStep)
	route("GET /api/v1/incidents/{id}/postmortem", s.handlePostmortemGet)
	route("PUT /api/v1/incidents/{id}/postmortem", s.handlePostmortemPut)
	route("POST /api/v1/approvals/{aid}/decision", s.handleApprovalDecision)
	route("GET /api/v1/approvals", s.handleApprovalsList)
	route("GET /api/v1/settings", s.handleSettingsGet)
	route("PUT /api/v1/settings", s.handleSettingsPut)
	// U6 mentor (guard token; unlock/lock tanpa guard).
	s.mentorRoute("POST /api/v1/mentor/unlock", false, s.handleMentorUnlock)
	s.mentorRoute("POST /api/v1/mentor/lock", false, s.handleMentorLock)
	s.mentorRoute("GET /api/v1/mentor/timeline", true, s.handleMentorTimeline)
	s.mentorRoute("POST /api/v1/mentor/inject/traffic", true, s.handleMentorInjectTraffic)
	s.mentorRoute("GET /api/v1/mentor/snapshots", true, s.handleSnapshotList)
	s.mentorRoute("POST /api/v1/mentor/snapshot/save", true, s.handleSnapshotSave)
	s.mentorRoute("POST /api/v1/mentor/snapshot/load", true, s.handleSnapshotLoad)
	s.mentorRoute("GET /api/v1/mentor/sessions", true, s.handleMentorSessions)
	s.mentorRoute("GET /api/v1/mentor/session-summary", true, s.handleMentorSessionSummary)
	s.mentorRoute("GET /api/v1/mentor/assessment", true, s.handleMentorAssessmentGet)
	s.mentorRoute("PUT /api/v1/mentor/assessment", true, s.handleMentorAssessmentPut)
	s.mentorRoute("GET /api/v1/mentor/export", true, s.handleMentorExport)
	// U6 belajar.
	route("GET /api/v1/learn/curriculum", s.handleCurriculum)
	route("GET /api/v1/learn/glossary", s.handleGlossary)
	route("GET /api/v1/learn/progress", s.handleLearnProgressGet)
	route("PUT /api/v1/learn/progress", s.handleLearnProgressPut)
	s.mux.HandleFunc("/", s.handleWeb)
}

func (s *server) handler() http.Handler {
	// Eksekusi: nonceCSP -> hostAllowlist -> mutationGuard -> mux.
	// nonceCSP paling luar agar SETIAP respons (termasuk 403) ber-CSP.
	h := http.Handler(s.mux)
	h = mutationGuard(h, s.cfg.token, s.cfg.port)
	h = hostAllowlist(h, s.cfg.port)
	h = nonceCSP(h, s.cfg.port)
	return h
}

// ---------- API ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	// Marshal dulu: nilai NaN/Inf membuat Marshal gagal — tanpa ini server
	// mengirim 200 dengan body kosong (ditemukan via TestSeriesShapeDemo).
	body, err := json.Marshal(v)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Encode gagal", "nilai tak dapat di-JSON-kan")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// RFC 9457 application/problem+json.
func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	writeProblemFields(w, status, title, detail, nil)
}

// writeProblemFields: varian dengan rincian validasi per-field (opsional).
func writeProblemFields(w http.ResponseWriter, status int, title, detail string, errs []fieldError) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	body := map[string]any{
		"type":   "https://payslo-lab.local/problems/request-rejected",
		"title":  title,
		"status": status,
		"detail": detail,
	}
	if len(errs) > 0 {
		body["errors"] = errs
	}
	_ = json.NewEncoder(w).Encode(body)
}

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	mode := "trainee"
	if s.mentor.check(r.Header.Get("X-Mentor-Token")) {
		mode = "mentor"
	}
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{
			"mode":        mode,
			"traineeName": nil,
			"scenario":    nil,
			"labUp":       true,
			"checkedAt":   "2026-10-16T07:00:00Z",
		})
		return
	}
	// U0 live: mode default trainee; labUp dari probe stack cepat.
	// Skenario/nama trainee menyusul di fase berikutnya.
	st := probeStack()
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":        mode,
		"traineeName": nil,
		"scenario":    nil,
		"labUp":       st.labUp,
		"checkedAt":   rfc3339(time.Now()),
	})
}

func (s *server) handleStack(w http.ResponseWriter, r *http.Request) {
	if s.cfg.demo {
		writeJSON(w, http.StatusOK, map[string]any{
			"labUp":     true,
			"checkedAt": "2026-10-16T07:00:00Z",
			"services": []any{
				map[string]any{"name": "postgres", "state": "up"},
				map[string]any{"name": "gateway-api", "state": "up"},
				map[string]any{"name": "envoy", "state": "up"},
			},
		})
		return
	}
	st := probeStack()
	svcs := make([]any, 0, len(st.services))
	for _, svc := range st.services {
		svcs = append(svcs, map[string]any{"name": svc.name, "state": svc.state})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"labUp":     st.labUp,
		"checkedAt": rfc3339(time.Now()),
		"services":  svcs,
	})
}

// ---------- web statis + injeksi token ----------

func (s *server) handleWeb(w http.ResponseWriter, r *http.Request) {
	if s.cfg.devProxy != "" && proxyPrefix(r.URL.Path) {
		s.proxyToVite(w, r)
		return
	}
	// U6: /mentor hanya untuk mode mentor. Trainee -> 403 (SPA tidak pernah
	// dihidupkan; menu /mentor juga tidak dirender UI).
	if r.URL.Path == "/mentor" || strings.HasPrefix(r.URL.Path, "/mentor/") {
		if !s.mentor.check(r.Header.Get("X-Mentor-Token")) {
			writeProblem(w, http.StatusForbidden, "Mode mentor diperlukan", "buka mode mentor dengan PIN dulu")
			return
		}
	}
	// SPA fallback: semua path non-aset menyajikan index.html (routing di
	// klien). /api/* tidak sampai sini (mux lebih spesifik menang).
	rel := "index.html"
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		rel = strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	}
	raw, err := s.readWeb(rel)
	if err != nil {
		if os.IsNotExist(err) {
			writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "jalankan `make ui-build` dulu")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "Gagal baca aset", "aset UI tidak terbaca")
		return
	}
	if r.URL.Path == "/" || strings.HasSuffix(rel, ".html") {
		nonce, _ := r.Context().Value(nonceKey).(string)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(injectToken(raw, s.cfg.token, nonce))
		return
	}
	// Aset immutable Vite (hash di nama file): cache lama.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, path.Base(rel), time.Time{}, strings.NewReader(string(raw)))
}

func proxyPrefix(p string) bool {
	return strings.HasPrefix(p, "/src/") ||
		strings.HasPrefix(p, "/@vite/") ||
		strings.HasPrefix(p, "/@react-refresh") ||
		strings.HasPrefix(p, "/node_modules/")
}

func (s *server) proxyToVite(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(s.cfg.devProxy)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "dev-proxy buruk", "URL --dev-proxy tidak valid")
		return
	}
	httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
}

// readWeb: embed (default) kecuali --web-dir dioverride (dev/test).
// rel: "index.html" atau "assets/<hash>.js".
func (s *server) readWeb(rel string) ([]byte, error) {
	if s.cfg.webDir != "" {
		raw, err := os.ReadFile(path.Join(s.cfg.webDir, rel))
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
	raw, err := fs.ReadFile(webDist, "web/dist/"+rel)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// injectToken menyisipkan token ke memori JS (bukan localStorage) lewat
// script nonce per-request. Penanda: </body> terakhir.
func injectToken(html []byte, token, nonce string) []byte {
	tok, _ := json.Marshal(token)
	tag := `<script nonce="` + nonce + `">window.__LAB_TOKEN__=` + string(tok) + `</script>`
	s := string(html)
	idx := strings.LastIndex(s, "</body>")
	if idx < 0 {
		return html
	}
	return []byte(s[:idx] + tag + s[idx:])
}
