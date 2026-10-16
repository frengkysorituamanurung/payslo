package main

// Mode mentor U6: gate PIN (MENTOR_PIN) + rate limit, timeline aksi trainee
// (audit PaySLO + audit lab serve, digabung urut waktu), injeksi traffic,
// snapshot save/load, penilaian manual, ringkasan sesi, ekspor laporan.
//
// JUJUR: gate ini PEMBATAS SPOILER, bukan keamanan. PIN diketahui siapa
// saja yang membaca repo/.env; trainee tetap bisa curl admin bank-sim
// langsung (honor system). Dokumentasi: docs/mentor-guide.md.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	mentorMaxFails  = 5
	mentorLockSecs  = 300 // jeda 5 menit setelah 5 salah
	mentorTokenLen  = 32
	snapDirName     = "snapshots"
	maxMentorResult = 500
)

var (
	errMentorDisabled = errors.New("MENTOR_PIN tidak diatur; mode mentor nonaktif")
	errMentorWrongPIN = errors.New("PIN salah")
	errMentorLocked   = errors.New("terlalu banyak percobaan; coba lagi nanti")
	errMentorSnapshot = errors.New("snapshot gagal")
)

// ------------------------------------------------------------------ gate

type mentorGate struct {
	mu          sync.Mutex
	pin         string // "" = nonaktif
	fails       int
	lockedUntil time.Time
	token       string // token aktif; "" = tidak ada
}

func newMentorGate(pin string) *mentorGate {
	return &mentorGate{pin: pin}
}

func (g *mentorGate) enabled() bool { return g.pin != "" }

// unlock memeriksa PIN + rate limit. Berhasil -> token baru (lama dicabut).
// 5x salah -> jeda 5 menit BERIKUTNYA (percobaan ke-5 masih 401; ke-6 -> 429).
func (g *mentorGate) unlock(pin string) (token string, retryAfter int, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pin == "" {
		return "", 0, errMentorDisabled
	}
	now := time.Now()
	if now.Before(g.lockedUntil) {
		return "", int(g.lockedUntil.Sub(now).Seconds()) + 1, errMentorLocked
	}
	if pin != g.pin {
		g.fails++
		if g.fails >= mentorMaxFails {
			g.fails = 0
			g.lockedUntil = now.Add(mentorLockSecs * time.Second)
			g.token = ""
			// Kunci berlaku untuk percobaan berikutnya; ini tetap "PIN salah".
			return "", 0, errMentorWrongPIN
		}
		return "", 0, errMentorWrongPIN
	}
	g.fails = 0
	g.lockedUntil = time.Time{}
	g.token = randToken(mentorTokenLen)
	return g.token, 0, nil
}

// lock mencabut token (keluar mode mentor).
func (g *mentorGate) lock() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = ""
	g.fails = 0
}

// check memvalidasi token request (header X-Mentor-Token).
func (g *mentorGate) check(token string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.token != "" && token == g.token
}

func randToken(n int) string {
	b := make([]byte, n)
	if _, err := randRead(b); err != nil {
		return strings.Repeat("x", n)
	}
	return hexEncode(b)
}

func randRead(b []byte) (int, error) { return rand.Read(b) }

func hexEncode(b []byte) string { return hex.EncodeToString(b) }

func readAllLimit(r io.Reader, n int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, n))
}

// readDirNames: nama entri direktori (bukan rekursif).
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// readSmallFile: <dir>/<name>/<file> untuk meta snapshot (maks 1 MiB).
func readSmallFile(dir, name, file string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, name, file))
}

// ------------------------------------------------------------------ timeline

type mentorTimelineEntry struct {
	TS     string `json:"ts"`
	Source string `json:"source"` // payslo | lab
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

// mergeTimeline menggabungkan audit PaySLO + audit lab serve, urut naik per ts.
// Pengujian: u6_test.go (data berselang-seling, urut benar).
func mergeTimeline(payslo []mentorTimelineEntry, lab []mentorTimelineEntry, limit int) []mentorTimelineEntry {
	all := make([]mentorTimelineEntry, 0, len(payslo)+len(lab))
	all = append(all, payslo...)
	all = append(all, lab...)
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].TS != all[j].TS {
			return all[i].TS < all[j].TS
		}
		// Deterministik: payslo sebelum lab bila ts sama.
		return all[i].Source < all[j].Source
	})
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all
}

func labAuditToTimeline(entries []auditEntry) []mentorTimelineEntry {
	out := make([]mentorTimelineEntry, 0, len(entries))
	for _, e := range entries {
		detail := e.Target
		if e.Detail != "" {
			if detail != "" {
				detail += " "
			}
			detail += e.Detail
		}
		if e.Result != "" && e.Result != "ok" {
			detail += " [" + e.Result + "]"
			if e.Error != "" {
				detail += ": " + e.Error
			}
		}
		out = append(out, mentorTimelineEntry{
			TS: e.TS, Source: "lab", Actor: e.Actor, Action: e.Action, Detail: detail,
		})
	}
	return out
}

func (c *paysloClient) getAudit(ctx context.Context, limit int) ([]mentorTimelineEntry, error) {
	code, body, err := c.get(ctx, "/audit?limit="+strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("payslo HTTP %d", code)
	}
	var decoded struct {
		Entries []struct {
			TS      string `json:"ts"`
			Actor   string `json:"actor"`
			Action  string `json:"action"`
			Details string `json:"details"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	out := make([]mentorTimelineEntry, 0, len(decoded.Entries))
	for _, e := range decoded.Entries {
		out = append(out, mentorTimelineEntry{
			TS: e.TS, Source: "payslo", Actor: e.Actor, Action: e.Action, Detail: e.Details,
		})
	}
	return out, nil
}

// ------------------------------------------------------------------ snapshot

type snapshotMeta struct {
	Name       string          `json:"name"`
	SavedAt    string          `json:"savedAt"`
	DataTables int             `json:"dataTables"`
	Sources    json.RawMessage `json:"sources"` // banks/merchant/routing/release (live) atau demo
	// Mode demo: sumber = {routing, release, traffic}.
	Demo *demoSnapshotState `json:"demo,omitempty"`
}

type demoSnapshotState struct {
	Routing routingConfig `json:"routing"`
	Release string        `json:"release"`
	Traffic float64       `json:"traffic"`
	SavedAt string        `json:"savedAt"`
}

func (s *server) snapshotDir() string {
	return filepath.Join(s.cfg.repoRoot, "payslo-workspace", snapDirName)
}

func snapshotNameOK(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for i, c := range name {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		if i == 0 {
			return false
		}
		return false
	}
	return true
}

// liveSnapshots: list/save/load memakai pg_dump/psql + profil HTTP (cermin
// `lab snapshot` di crates/lab). Demo: in-memory (uji tanpa Docker).
func (s *server) snapshotSave(name string) (*snapshotMeta, error) {
	if s.cfg.demo {
		return s.snapshotSaveDemo(name)
	}
	dir := filepath.Join(s.snapshotDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// Dump data-only (buku migrasi disisakan).
	args := []string{"compose", "--profile", "traffic", "--profile", "obs", "exec", "-T", "postgres",
		"pg_dump", "-U", "lab_app", "-d", "payslo_lab", "--data-only", "-T", "_sqlx_migrations"}
	out, err := s.composeCmdOutput(args)
	if err != nil {
		return nil, fmt.Errorf("%w: pg_dump: %s", errMentorSnapshot, err)
	}
	tables := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "COPY ") && strings.HasSuffix(line, " FROM stdin;") {
			tables++
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "db.sql"), out, 0o644); err != nil {
		return nil, err
	}
	sources, err := s.snapshotCollectSources()
	if err != nil {
		return nil, err
	}
	meta := &snapshotMeta{
		Name: name, SavedAt: time.Now().UTC().Format(time.RFC3339),
		DataTables: tables, Sources: sources,
	}
	raw, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), raw, 0o644); err != nil {
		return nil, err
	}
	return meta, nil
}

// snapshotCollectSources mengambil profil chaos live: 3 bank, merchant, routing, release.
func (s *server) snapshotCollectSources() ([]byte, error) {
	get := func(url string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.httpClient().Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := readAllLimit(resp.Body, 1<<16)
		return body, nil
	}
	sources := map[string]any{}
	for name, url := range s.bankURLs {
		body, err := get(url + "/admin/profile")
		if err != nil {
			return nil, fmt.Errorf("bank %s: %w", name, err)
		}
		sources["bank_"+name] = json.RawMessage(body)
	}
	if body, err := get(s.merchantURL + "/admin/mode"); err == nil {
		sources["merchant_mode"] = json.RawMessage(body)
	}
	if body, err := get(s.adminURL + "/admin/routing"); err == nil {
		sources["routing"] = json.RawMessage(body)
	}
	if body, err := get(s.adminURL + "/admin/release"); err == nil {
		sources["release"] = json.RawMessage(body)
	}
	return json.Marshal(sources)
}

func (s *server) snapshotLoad(name string) (*snapshotMeta, []string, error) {
	if s.cfg.demo {
		return s.snapshotLoadDemo(name)
	}
	dir := filepath.Join(s.snapshotDir(), name)
	dbSQL, err := os.ReadFile(filepath.Join(dir, "db.sql"))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: snapshot %q tidak ada", errMentorSnapshot, name)
	}
	metaRaw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, nil, err
	}
	var meta snapshotMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, nil, err
	}
	restored := []string{}
	// Replay DB: TRUNCATE dulu (dump hanya INSERT data).
	full := "TRUNCATE webhook_delivery, callback_log, transactions CASCADE;\n" + string(dbSQL)
	if err := s.composePSQL(full); err != nil {
		return nil, nil, fmt.Errorf("%w: psql: %s", errMentorSnapshot, err)
	}
	restored = append(restored, "db")
	// Profil chaos.
	if err := s.snapshotRestoreSources(meta.Sources); err != nil {
		return nil, nil, err
	}
	restored = append(restored, "banks", "merchant", "routing", "release")
	// Lepas lock DB yang ditahan lab.
	s.composePSQL("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE query LIKE '%ACCESS EXCLUSIVE%' AND pid <> pg_backend_pid();")
	return &meta, restored, nil
}

func (s *server) snapshotRestoreSources(raw json.RawMessage) error {
	var sources map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sources); err != nil {
		return err
	}
	put := func(url string, body []byte) error {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(body)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.httpClient().Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("PUT %s -> HTTP %d", url, resp.StatusCode)
		}
		return nil
	}
	for name, url := range s.bankURLs {
		if body, ok := sources["bank_"+name]; ok {
			if err := put(url+"/admin/profile", body); err != nil {
				return err
			}
		}
	}
	if body, ok := sources["merchant_mode"]; ok {
		if err := put(s.merchantURL+"/admin/mode", body); err != nil {
			return err
		}
	}
	if body, ok := sources["routing"]; ok {
		if err := put(s.adminURL+"/admin/routing", body); err != nil {
			return err
		}
	}
	if body, ok := sources["release"]; ok {
		if err := put(s.adminURL+"/admin/release", body); err != nil {
			return err
		}
	}
	return nil
}

// demo snapshots (uji tanpa Docker).
func (s *server) snapshotSaveDemo(name string) (*snapshotMeta, error) {
	dc := s.control.(*demoControl)
	dc.mu.Lock()
	defer dc.mu.Unlock()
	savedAt := time.Now().UTC().Format(time.RFC3339)
	meta := &snapshotMeta{
		Name:       name,
		SavedAt:    savedAt,
		DataTables: 3,
		Sources:    json.RawMessage(`{"demo":true}`),
		Demo: &demoSnapshotState{
			Routing: dc.routing, Release: dc.release, Traffic: dc.traffic,
			SavedAt: savedAt,
		},
	}
	s.demoSnapshotsMu.Lock()
	s.demoSnapshots[name] = meta.Demo
	s.demoSnapshotsMu.Unlock()
	return meta, nil
}

func (s *server) snapshotLoadDemo(name string) (*snapshotMeta, []string, error) {
	s.demoSnapshotsMu.Lock()
	defer s.demoSnapshotsMu.Unlock()
	state, ok := s.demoSnapshots[name]
	if !ok {
		return nil, nil, fmt.Errorf("%w: snapshot %q tidak ada", errMentorSnapshot, name)
	}
	dc := s.control.(*demoControl)
	dc.mu.Lock()
	dc.routing = state.Routing
	dc.release = state.Release
	dc.traffic = state.Traffic
	dc.mu.Unlock()
	meta := &snapshotMeta{
		Name: name, SavedAt: time.Now().UTC().Format(time.RFC3339),
		DataTables: 3, Sources: json.RawMessage(`{"demo":true}`),
	}
	return meta, []string{"routing", "release", "traffic"}, nil
}

func (s *server) demoSnapshotList() []snapshotMeta {
	s.demoSnapshotsMu.Lock()
	defer s.demoSnapshotsMu.Unlock()
	out := []snapshotMeta{}
	for name, st := range s.demoSnapshots {
		out = append(out, snapshotMeta{
			Name: name, SavedAt: st.SavedAt, DataTables: 3,
			Sources: json.RawMessage(`{"demo":true}`),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt > out[j].SavedAt })
	return out
}

// composeCmdOutput menjalankan docker compose di repoRoot dan mengembalikan stdout.
func (s *server) composeCmdOutput(args []string) ([]byte, error) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = s.cfg.repoRoot
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// composePSQL menjalankan psql via docker compose exec dengan SQL dari stdin.
func (s *server) composePSQL(sql string) error {
	cmd := exec.Command("docker", "compose", "--profile", "traffic", "--profile", "obs",
		"exec", "-T", "postgres", "psql", "-U", "lab_app", "-d", "payslo_lab", "-v", "ON_ERROR_STOP=1", "-q")
	cmd.Dir = s.cfg.repoRoot
	cmd.Stdin = strings.NewReader(sql)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ------------------------------------------------------------------ sesi trainee

// labSession cermin crates/lab session.rs (unix seconds -> RFC3339 di API).
type labSession struct {
	ScenarioID string      `json:"scenario_id"`
	StartedAt  int64       `json:"started_at"`
	EndedAt   *int64      `json:"ended_at"`
	HintsUsed  []string    `json:"hints_used"`
	Answers    map[string]string `json:"answers"`
	Actions    []struct {
		TS     int64  `json:"ts"`
		Cmd    string `json:"cmd"`
		Detail string `json:"detail"`
	} `json:"actions"`
}

func unixToRFC3339(t int64) string {
	return time.Unix(t, 0).UTC().Format(time.RFC3339)
}

func (s *server) listLabSessions() ([]labSession, error) {
	dir := filepath.Join(s.cfg.repoRoot, "payslo-workspace", "lab-sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []labSession{}, nil
		}
		return nil, err
	}
	out := []labSession{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var sess labSession
		if json.Unmarshal(raw, &sess) != nil {
			continue
		}
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out, nil
}

func (s *server) loadLabSession(scenarioID string) (*labSession, error) {
	dir := filepath.Join(s.cfg.repoRoot, "payslo-workspace", "lab-sessions")
	raw, err := os.ReadFile(filepath.Join(dir, scenarioID+".json"))
	if err != nil {
		return nil, err
	}
	var sess labSession
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}
