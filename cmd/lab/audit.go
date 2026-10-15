package main

// Audit log lab serve (U2, UI-SPEC U4).
//
// Setiap mutasi (POST/PUT) mencatat aktor, aksi, target, waktu, hasil.
// Penyimpanan: file JSONL append-only (satu objek per baris) + cache memori
// terbatas untuk pembacaan cepat. Mode --demo memakai memori saja (tanpa file),
// agar test tidak menyentuh disk.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const auditMemMax = 1000

type auditEntry struct {
	TS     string `json:"ts"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail,omitempty"`
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

type auditor struct {
	mu   sync.Mutex
	path string // kosong = memori saja
	mem  []auditEntry
	max  int
}

func newAuditor(path string) *auditor {
	return &auditor{path: path, max: auditMemMax}
}

// defaultAuditPath: file JSONL di bawah repo (gitignored).
func defaultAuditPath(repoRoot string) string {
	return filepath.Join(repoRoot, "payslo-workspace", "lab-audit.jsonl")
}

// record mencatat satu entri. Gagal tulis file tidak menggagalkan request
// (audit best-effort) tetapi tetap masuk memori agar terlihat di UI.
func (a *auditor) record(e auditEntry) {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if e.Actor == "" {
		e.Actor = "lab-console"
	}
	if e.Result == "" {
		e.Result = "ok"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mem = append(a.mem, e)
	if len(a.mem) > a.max {
		a.mem = a.mem[len(a.mem)-a.max:]
	}
	if a.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
}

// list mengembalikan entri terbaru dulu, maksimal limit.
func (a *auditor) list(limit int) []auditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	if limit <= 0 || limit > a.max {
		limit = a.max
	}
	out := make([]auditEntry, 0, limit)
	for i := len(a.mem) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, a.mem[i])
	}
	return out
}
