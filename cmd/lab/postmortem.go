package main

// Postmortem + pengaturan (U4): milik lab serve (PaySLO tak punya API ini).
// Disimpan sebagai JSON di payslo-workspace/lab-console/ (gitignored di
// level direktori? workspace milik trainee — catat di DECISIONS). Markdown
// di-render server-side secara deterministik (satu sumber untuk UI + ekspor).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ------------------------------------------------------------------ postmortem

type postmortemSections struct {
	Summary   string   `json:"summary"`
	Impact    string   `json:"impact"`
	Timeline  string   `json:"timeline"`
	RootCause string   `json:"rootCause"`
	WentWell  string   `json:"wentWell"`
	WentWrong string   `json:"wentWrong"`
	Actions   []string `json:"actions"`
}

type postmortemDoc struct {
	IncidentID string             `json:"incidentId"`
	Status     string             `json:"status"` // draft|published
	Sections   postmortemSections `json:"sections"`
	UpdatedAt  string             `json:"updatedAt"`
	UpdatedBy  string             `json:"updatedBy"`
	Markdown   string             `json:"markdown"`
	Exists     bool               `json:"exists"`
}

type postmortemStore struct {
	mu  sync.Mutex
	dir string
}

func (s *postmortemStore) pathFor(incidentID string) (string, error) {
	if !validIncidentID(incidentID) {
		return "", errIncidentNotFound
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	// ID tervalidasi [a-z0-9-] -> aman untuk nama berkas.
	return filepath.Join(s.dir, incidentID+".json"), nil
}

func blankPostmortem(incidentID string) postmortemDoc {
	return postmortemDoc{
		IncidentID: incidentID, Status: "draft",
		Sections: postmortemSections{}, Exists: false,
	}
}

func (s *postmortemStore) Get(incident incidentSummary, incidentID string) (postmortemDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.pathFor(incidentID)
	if err != nil {
		return postmortemDoc{}, err
	}
	doc := blankPostmortem(incidentID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			doc.Markdown = renderPostmortem(incident, doc)
			return doc, nil
		}
		return postmortemDoc{}, err
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return postmortemDoc{}, err
	}
	doc.IncidentID = incidentID
	doc.Exists = true
	doc.Markdown = renderPostmortem(incident, doc)
	return doc, nil
}

func (s *postmortemStore) Save(incidentID, status string, sections postmortemSections, by string) (postmortemDoc, error) {
	if err := validatePostmortem(status, sections); err != nil {
		return postmortemDoc{}, err
	}
	savePath, err := s.pathFor(incidentID)
	if err != nil {
		return postmortemDoc{}, err
	}
	doc := postmortemDoc{
		IncidentID: incidentID, Status: status, Sections: sections,
		UpdatedAt: rfc3339(time.Now()), UpdatedBy: by, Exists: true,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return postmortemDoc{}, err
	}
	if err := os.WriteFile(savePath, raw, 0o644); err != nil {
		return postmortemDoc{}, err
	}
	return doc, nil
}

// renderPostmortem: template Markdown deterministik (diuji: header + semua
// bagian + action items selalu hadir dalam urutan tetap).
func renderPostmortem(incident incidentSummary, doc postmortemDoc) string {
	var sb strings.Builder
	title := strings.TrimSpace(incident.Title)
	if title == "" {
		title = doc.IncidentID
	}
	sb.WriteString("# Postmortem " + title + " (" + doc.IncidentID + ")\n\n")
	sb.WriteString("- Status: " + doc.Status + "\n")
	if incident.Slo != "" {
		sb.WriteString("- SLO: " + incident.Slo + "\n")
		sb.WriteString("- Service: " + incident.Service + "\n")
		sb.WriteString("- Severity: " + incident.Severity + "\n")
		sb.WriteString("- Status insiden: " + incident.Status + "\n")
	}
	s := doc.Sections
	section := func(name, body string) {
		sb.WriteString("\n## " + name + "\n\n")
		if strings.TrimSpace(body) == "" {
			sb.WriteString("_belum diisi_\n")
		} else {
			sb.WriteString(strings.TrimSpace(body) + "\n")
		}
	}
	section("Ringkasan", s.Summary)
	section("Dampak", s.Impact)
	section("Kronologi", s.Timeline)
	section("Akar masalah", s.RootCause)
	section("Yang berjalan baik", s.WentWell)
	section("Yang berjalan buruk", s.WentWrong)
	sb.WriteString("\n## Action items\n\n")
	if len(s.Actions) == 0 {
		sb.WriteString("_belum ada_\n")
	} else {
		for _, a := range s.Actions {
			sb.WriteString("- [ ] " + strings.TrimSpace(a) + "\n")
		}
	}
	return sb.String()
}

// ------------------------------------------------------------------ pengaturan

type labSettings struct {
	TraineeName string `json:"traineeName"`
}

type settingsStore struct {
	mu   sync.Mutex
	path string
}

func validatePostmortem(status string, sections postmortemSections) error {
	if status != "draft" && status != "published" {
		return fmt.Errorf("status harus draft atau published")
	}
	for _, text := range []string{sections.Summary, sections.Impact, sections.Timeline, sections.RootCause, sections.WentWell, sections.WentWrong} {
		if len(text) > 20_000 {
			return fmt.Errorf("bagian postmortem maks 20000 karakter")
		}
	}
	if len(sections.Actions) > 50 {
		return fmt.Errorf("action items maks 50")
	}
	for _, a := range sections.Actions {
		if len(a) > 500 {
			return fmt.Errorf("action item maks 500 karakter")
		}
	}
	return nil
}

func validateTraineeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 || strings.ContainsAny(name, "\r\n") {
		return "", fmt.Errorf("nama trainee 1..64 karakter tanpa baris baru")
	}
	return name, nil
}
func (s *settingsStore) Get() labSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return labSettings{}
	}
	var out labSettings
	if json.Unmarshal(raw, &out) != nil {
		return labSettings{}
	}
	return out
}

func (s *settingsStore) Set(name string) (labSettings, error) {
	name, err := validateTraineeName(name)
	if err != nil {
		return labSettings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return labSettings{}, err
	}
	out := labSettings{TraineeName: name}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return labSettings{}, err
	}
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		return labSettings{}, err
	}
	return out, nil
}

func (s *settingsStore) traineeName() (string, bool) {
	got := strings.TrimSpace(s.Get().TraineeName)
	return got, got != ""
}
