package main

// Progress + assessment store U6 (SQLite, modernc.org/sqlite — pure Go, tanpa
// cgo; dipin v1.38.1). File: <consoleDir>/lab.db. Tahan restart lab serve
// (data di disk, bukan memori). Akses sinkron (handler sudah berjalan di
// goroutine terpisah; volume lab kecil).

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type progressStore struct {
	mu sync.Mutex
	db *sql.DB
}

func openProgressStore(consoleDir string) (*progressStore, error) {
	if err := os.MkdirAll(consoleDir, 0o755); err != nil {
		return nil, fmt.Errorf("buat direktori console: %w", err)
	}
	path := filepath.Join(consoleDir, "lab.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("buka sqlite: %w", err)
	}
	stmt := `CREATE TABLE IF NOT EXISTS learn_progress (
        module TEXT NOT NULL,
        key   TEXT NOT NULL,
        done  INTEGER NOT NULL DEFAULT 0,
        updated_at TEXT NOT NULL,
        PRIMARY KEY (module, key)
    );
    CREATE TABLE IF NOT EXISTS mentor_assessment (
        scenario_id TEXT PRIMARY KEY,
        communication_score INTEGER,
        communication_note TEXT NOT NULL DEFAULT '',
        postmortem_score INTEGER,
        postmortem_note TEXT NOT NULL DEFAULT '',
        updated_at TEXT
    );`
	if _, err := db.Exec(stmt); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	return &progressStore{db: db}, nil
}

func (p *progressStore) Close() error { return p.db.Close() }

type learnProgressItem struct {
	Module string `json:"module"`
	Key    string `json:"key"`
	Done   bool   `json:"done"`
}

func (p *progressStore) setProgress(module, key string, done bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.Exec(
		`INSERT INTO learn_progress (module, key, done, updated_at) VALUES (?, ?, ?, ?)
         ON CONFLICT (module, key) DO UPDATE SET done = excluded.done, updated_at = excluded.updated_at`,
		module, key, boolToInt(done), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (p *progressStore) listProgress() ([]learnProgressItem, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rows, err := p.db.Query(`SELECT module, key, done FROM learn_progress WHERE done = 1 ORDER BY module, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []learnProgressItem{}
	for rows.Next() {
		var it learnProgressItem
		var done int
		if err := rows.Scan(&it.Module, &it.Key, &done); err != nil {
			return nil, err
		}
		it.Done = done == 1
		out = append(out, it)
	}
	return out, rows.Err()
}

type mentorAssessment struct {
	ScenarioID          string  `json:"scenarioId"`
	CommunicationScore *int    `json:"communicationScore,omitempty"`
	CommunicationNote  string  `json:"communicationNote"`
	PostmortemScore    *int    `json:"postmortemScore,omitempty"`
	PostmortemNote     string  `json:"postmortemNote"`
	UpdatedAt          *string `json:"updatedAt"`
}

func (p *progressStore) getAssessment(scenarioID string) (*mentorAssessment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var a mentorAssessment
	var comm, pm sql.NullInt64
	var updated sql.NullString
	err := p.db.QueryRow(
		`SELECT communication_score, communication_note, postmortem_score, postmortem_note, updated_at
         FROM mentor_assessment WHERE scenario_id = ?`, scenarioID).
		Scan(&comm, &a.CommunicationNote, &pm, &a.PostmortemNote, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.ScenarioID = scenarioID
	if comm.Valid {
		v := int(comm.Int64)
		a.CommunicationScore = &v
	}
	if pm.Valid {
		v := int(pm.Int64)
		a.PostmortemScore = &v
	}
	if updated.Valid {
		a.UpdatedAt = &updated.String
	}
	return &a, nil
}

func (p *progressStore) saveAssessment(a *mentorAssessment) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.db.Exec(
		`INSERT INTO mentor_assessment (scenario_id, communication_score, communication_note, postmortem_score, postmortem_note, updated_at)
         VALUES (?, ?, ?, ?, ?, ?)
         ON CONFLICT (scenario_id) DO UPDATE SET
           communication_score = excluded.communication_score,
           communication_note = excluded.communication_note,
           postmortem_score = excluded.postmortem_score,
           postmortem_note = excluded.postmortem_note,
           updated_at = excluded.updated_at`,
		a.ScenarioID, intPtr(a.CommunicationScore), a.CommunicationNote,
		intPtr(a.PostmortemScore), a.PostmortemNote, time.Now().UTC().Format(time.RFC3339))
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intPtr(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
