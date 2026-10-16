package main

// Halaman Belajar U6: kurikulum Markdown -> HTML tersanitasi (renderer aman
// di markdown.go), checklist per modul (daftar Aktivitas + Latihan),
// glosarium statis, progres checklist di SQLite lab serve (tahan restart).

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ------------------------------------------------------------------ kurikulum

type checklistItem struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Section string `json:"section"` // aktivitas | latihan
}

type curriculumModule struct {
	Day   string          `json:"day"`
	Title string          `json:"title"`
	HTML  string          `json:"html"`
	Items []checklistItem `json:"items"`
}

var dayFileRe = regexp.MustCompile(`^day-(\d{2})\.md$`)

// loadCurriculum membaca curriculum/day-*.md dari repo root, merender HTML
// aman, dan mengekstrak checklist (daftar di bawah ## Aktivitas / ## Latihan).
// Direktori tidak ada -> list kosong (degradasi anggun, bukan 404).
func loadCurriculum(repoRoot string) ([]curriculumModule, error) {
	dir := filepath.Join(repoRoot, "curriculum")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []curriculumModule{}, nil
		}
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() || !dayFileRe.MatchString(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("curriculum/day-*.md tidak ditemukan di %s", dir)
	}
	out := []curriculumModule{}
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		mod := curriculumModule{
			Day:   strings.TrimSuffix(name, ".md"),
			Title: curriculumTitle(raw),
			HTML:  renderMarkdown(string(raw)),
			Items: curriculumChecklist(raw),
		}
		out = append(out, mod)
	}
	return out, nil
}

// curriculumTitle: heading pertama (# ...) atau fallback nama berkas.
func curriculumTitle(raw []byte) string {
	for _, line := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	return "Kurikulum"
}

// curriculumChecklist: item daftar di bawah "## Aktivitas" dan "## Latihan".
func curriculumChecklist(raw []byte) []checklistItem {
	lines := strings.Split(string(raw), "\n")
	items := []checklistItem{}
	section := ""
	n := 0
	flush := func() {}
	_ = flush
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			switch strings.TrimSpace(t[3:]) {
			case "Aktivitas":
				section = "aktivitas"
			case "Latihan":
				section = "latihan"
			default:
				section = ""
			}
			continue
		}
		if section == "" {
			continue
		}
		rest := orderedItem(t)
		if rest == "" {
			rest = strings.TrimPrefix(t, "- ")
			if rest == t {
				continue
			}
		}
		if strings.TrimSpace(rest) == "" {
			continue
		}
		n++
		items = append(items, checklistItem{
			Key:     section + "-" + strconvItoa(n),
			Label:   strings.TrimSpace(rest),
			Section: section,
		})
	}
	if items == nil {
		items = []checklistItem{}
	}
	return items
}

func strconvItoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// ------------------------------------------------------------------ glosarium

type glossaryTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

var glossaryTerms = []glossaryTerm{
	{"SLI", "Service Level Indicator: ukuran kuantitatif layanan (mis. rasio sukses = good/total). 0% dari 'tidak ada data' berbeda dari 0% dari sampel."},
	{"SLO", "Service Level Objective: target SLI dalam window waktu (mis. 99.9% sukses dalam 30 hari). Dilanggar -> error budget habis."},
	{"Error budget", "1 - SLO. Mis. SLO 99.9% -> budget 0.1% kegagalan yang 'diizinkan' dalam window."},
	{"Burn rate", "Laju penghabisan error budget. Fast-burn (window pendek) = page; slow-burn (window panjang) = ticket."},
	{"TTD", "Time to Detect: waktu dari insiden mulai sampai trainee bertindak pertama (diukur dari aksi tercatat)."},
	{"TTM", "Time to Mitigate: waktu sampai insiden teratasi (mitigasi terbukti via check)."},
	{"P2 (lab)", "HTTP 200 TIDAK berarti pembayaran sukses: 200 = diterima gateway; status final lewat callback bank."},
	{"Golden signals", "Rate, Error, Duration, Saturation — empat sinyal utama untuk membaca kesehatan layanan."},
	{"Runbook", "Dokumentasi langkah penanganan insiden: diagnostic (otomatis), remediation (butuh approval), manual."},
	{"Idempotency key", "Kunci agar request ulang tidak dobel efeknya. Di lab: dicatat gateway, ditegakkan via UNIQUE(merchant_id, order_id)."},
	{"Ambiguous failure", "Bank memproses sukses tapi respons ke gateway timeout — status akhir hanya diketahui via callback."},
	{"Dedup callback", "LAB-FLAW lab: callback duplikat TIDAK di-dedup gateway (skenario 03) — saldo merchant bisa ganda."},
}

// ------------------------------------------------------------------ handler

func (s *server) handleCurriculum(w http.ResponseWriter, r *http.Request) {
	modules, err := loadCurriculum(s.cfg.repoRoot)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "Kurikulum tidak ditemukan", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"modules": modules})
}

func (s *server) handleGlossary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"terms": glossaryTerms})
}

func (s *server) handleLearnProgressGet(w http.ResponseWriter, r *http.Request) {
	items, err := s.progress.listProgress()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Progres tak terbaca", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) handleLearnProgressPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Module string `json:"module"`
		Key    string `json:"key"`
		Done   bool   `json:"done"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	if body.Module == "" || body.Key == "" {
		validationProblem(w, []fieldError{{"module", "module dan key wajib"}})
		return
	}
	if err := s.progress.setProgress(body.Module, body.Key, body.Done); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Progres gagal disimpan", err.Error())
		return
	}
	actor := actorOf(r)
	s.audit.record(auditEntry{Actor: actor, Action: "learn.progress", Target: body.Module + "/" + body.Key, Detail: fmt.Sprintf("done=%v", body.Done)})
	items, err := s.progress.listProgress()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Progres tak terbaca", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
