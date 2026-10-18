package main

// Handler HTTP U6: mentor (gate, timeline, inject, snapshot, sesi, penilaian,
// ekspor) + learn (kurikulum, glosarium, progres). Semua /api/v1/mentor/*
// (kecuali unlock/lock) di-wrap mentorGuard: wajib X-Mentor-Token valid.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// mentorRoute mendaftarkan route mentor; guarded=true mewajibkan token
// mentor valid (unlock/lock = false).
func (s *server) mentorRoute(pattern string, guarded bool, h http.HandlerFunc) {
	s.routes = append(s.routes, pattern)
	if !guarded {
		s.mux.HandleFunc(pattern, h)
		return
	}
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if !s.mentor.check(r.Header.Get("X-Mentor-Token")) {
			writeProblem(w, http.StatusForbidden, "Mode mentor diperlukan", "endpoint ini hanya untuk mode mentor (PIN)")
			return
		}
		h(w, r)
	})
}

// ------------------------------------------------------------------ unlock/lock

func (s *server) handleMentorUnlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Pin string `json:"pin"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	token, retryAfter, err := s.mentor.unlock(body.Pin)
	if err != nil {
		switch err {
		case errMentorDisabled:
			writeProblem(w, http.StatusForbidden, "Mode mentor nonaktif", err.Error())
		case errMentorLocked:
			writeProblem(w, http.StatusTooManyRequests, "Jeda percobaan", err.Error())
		default:
			writeProblem(w, http.StatusUnauthorized, "PIN salah", err.Error())
		}
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		}
		return
	}
	s.audit.record(auditEntry{Actor: "mentor", Action: "mentor.unlock", Target: "session"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": token})
}

func (s *server) handleMentorLock(w http.ResponseWriter, r *http.Request) {
	if !s.mentor.check(r.Header.Get("X-Mentor-Token")) {
		writeProblem(w, http.StatusForbidden, "Mode mentor diperlukan", "token mentor tidak valid")
		return
	}
	s.mentor.lock()
	s.audit.record(auditEntry{Actor: "mentor", Action: "mentor.lock", Target: "session"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ------------------------------------------------------------------ timeline

func (s *server) handleMentorTimeline(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxMentorResult {
			writeProblem(w, http.StatusBadRequest, "Param buruk", "limit harus 1..500")
			return
		}
		limit = n
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	paysloEntries, err := s.incidents.paysloAudit(ctx, limit)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "PaySLO tak terjangkau", err.Error())
		return
	}
	labEntries := labAuditToTimeline(s.audit.list(limit))
	entries := mergeTimeline(paysloEntries, labEntries, limit)
	if entries == nil {
		entries = []mentorTimelineEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// ------------------------------------------------------------------ inject

func (s *server) handleMentorInjectTraffic(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RatePerSec int  `json:"ratePerSec"`
		Secs       *int `json:"secs"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	if body.RatePerSec < minTrafficRate || body.RatePerSec > maxTrafficRate {
		validationProblem(w, []fieldError{{"ratePerSec", "ratePerSec harus integer 1..100"}})
		return
	}
	actor := actorOf(r)
	st, err := s.control.SetTraffic(r.Context(), float64(body.RatePerSec))
	if err != nil {
		s.audit.record(auditEntry{Actor: actor, Action: "mentor.inject", Target: "traffic", Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadGateway, "Docker gagal", err.Error())
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "mentor.inject", Target: "traffic", Detail: fmt.Sprintf("rate=%d/s", body.RatePerSec)})
	// Revert latar belakang setelah N detik (best-effort).
	if body.Secs != nil && *body.Secs > 0 {
		prev := st.RatePerSec
		go func() {
			time.Sleep(time.Duration(*body.Secs) * time.Second)
			if _, err := s.control.SetTraffic(context.Background(), prevOr2(prev)); err != nil {
				return
			}
			s.audit.record(auditEntry{Actor: "mentor", Action: "mentor.inject.revert", Target: "traffic", Detail: fmt.Sprintf("rate=%v/s", prevOr2(prev))})
		}()
	}
	writeJSON(w, http.StatusOK, st)
}

func prevOr2(v *float64) float64 {
	if v == nil {
		return 2
	}
	return *v
}

// ------------------------------------------------------------------ snapshot

func (s *server) handleSnapshotList(w http.ResponseWriter, r *http.Request) {
	type info struct {
		Name        string `json:"name"`
		SavedAt     string `json:"savedAt"`
		DataTables  int    `json:"dataTables"`
		Overwritten bool   `json:"overwritten"`
	}
	if s.cfg.demo {
		metas := s.demoSnapshotList()
		out := make([]info, 0, len(metas))
		for _, m := range metas {
			out = append(out, info{Name: m.Name, SavedAt: m.SavedAt, DataTables: m.DataTables})
		}
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": out})
		return
	}
	dir := s.snapshotDir()
	entries, err := readDirNames(dir)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": []info{}})
		return
	}
	out := []info{}
	for _, name := range entries {
		raw, err := readSmallFile(dir, name, "meta.json")
		if err != nil {
			continue
		}
		var meta snapshotMeta
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		out = append(out, info{Name: meta.Name, SavedAt: meta.SavedAt, DataTables: meta.DataTables})
	}
	// Terbaru dulu.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": out})
}

func (s *server) handleSnapshotSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	if !snapshotNameOK(body.Name) {
		validationProblem(w, []fieldError{{"name", "name harus slug a-z0-9- (maks 64)"}})
		return
	}
	actor := actorOf(r)
	meta, err := s.snapshotSave(body.Name)
	if err != nil {
		s.audit.record(auditEntry{Actor: actor, Action: "snapshot.save", Target: body.Name, Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadGateway, "Snapshot gagal", err.Error())
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "snapshot.save", Target: body.Name, Detail: fmt.Sprintf("tables=%d", meta.DataTables)})
	writeJSON(w, http.StatusOK, meta)
}

func (s *server) handleSnapshotLoad(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	if !snapshotNameOK(body.Name) {
		validationProblem(w, []fieldError{{"name", "name harus slug a-z0-9- (maks 64)"}})
		return
	}
	actor := actorOf(r)
	meta, restored, err := s.snapshotLoad(body.Name)
	if err != nil {
		s.audit.record(auditEntry{Actor: actor, Action: "snapshot.load", Target: body.Name, Result: "error", Error: err.Error()})
		if strings.Contains(err.Error(), "tidak ada") {
			writeProblem(w, http.StatusNotFound, "Snapshot tidak ditemukan", err.Error())
			return
		}
		writeProblem(w, http.StatusBadGateway, "Snapshot gagal", err.Error())
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "snapshot.load", Target: body.Name, Detail: strings.Join(restored, ",")})
	writeJSON(w, http.StatusOK, map[string]any{"name": meta.Name, "restored": restored})
}

// ------------------------------------------------------------------ sesi + penilaian

func (s *server) handleMentorSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.listLabSessions()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Sesi tak terbaca", err.Error())
		return
	}
	type info struct {
		ScenarioID string  `json:"scenarioId"`
		StartedAt  string  `json:"startedAt"`
		EndedAt    *string `json:"endedAt"`
	}
	out := make([]info, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, info{
			ScenarioID: sess.ScenarioID,
			StartedAt:  unixToRFC3339(sess.StartedAt),
			EndedAt:    optRFC3339(sess.EndedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func optRFC3339(t *int64) *string {
	if t == nil {
		return nil
	}
	s := unixToRFC3339(*t)
	return &s
}

func (s *server) handleMentorSessionSummary(w http.ResponseWriter, r *http.Request) {
	scenarioID := strings.TrimSpace(r.URL.Query().Get("scenario"))
	if scenarioID == "" {
		writeProblem(w, http.StatusBadRequest, "Param buruk", "scenario wajib")
		return
	}
	sess, err := s.loadLabSession(scenarioID)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "Sesi tidak ditemukan", "jalankan `lab scenario start "+scenarioID+"` dulu")
		return
	}
	ttd := (*int64)(nil)
	if len(sess.Actions) > 0 {
		first := sess.Actions[0].TS
		for _, a := range sess.Actions {
			if a.TS < first {
				first = a.TS
			}
		}
		v := first - sess.StartedAt
		ttd = &v
	}
	assessment, err := s.progress.getAssessment(scenarioID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Penilaian tak terbaca", err.Error())
		return
	}
	if assessment == nil {
		assessment = &mentorAssessment{ScenarioID: scenarioID}
	}
	summary := map[string]any{
		"scenarioId":  sess.ScenarioID,
		"traineeName": s.settings.Get().TraineeName,
		"startedAt":   unixToRFC3339(sess.StartedAt),
		"endedAt":     optRFC3339(sess.EndedAt),
		"ttdSecs":     ttd,
		"actionCount": len(sess.Actions),
		"answerCount": len(sess.Answers),
		"hintsUsed":   len(sess.HintsUsed),
		"actions":     sess.Actions,
		"answers":     sess.Answers,
		"assessment":  assessment,
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *server) handleMentorAssessmentGet(w http.ResponseWriter, r *http.Request) {
	scenarioID := strings.TrimSpace(r.URL.Query().Get("scenario"))
	if scenarioID == "" {
		writeProblem(w, http.StatusBadRequest, "Param buruk", "scenario wajib")
		return
	}
	assessment, err := s.progress.getAssessment(scenarioID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Penilaian tak terbaca", err.Error())
		return
	}
	if assessment == nil {
		assessment = &mentorAssessment{ScenarioID: scenarioID}
	}
	writeJSON(w, http.StatusOK, assessment)
}

func (s *server) handleMentorAssessmentPut(w http.ResponseWriter, r *http.Request) {
	scenarioID := strings.TrimSpace(r.URL.Query().Get("scenario"))
	if scenarioID == "" {
		writeProblem(w, http.StatusBadRequest, "Param buruk", "scenario wajib")
		return
	}
	var body struct {
		CommunicationScore *int   `json:"communicationScore"`
		CommunicationNote  string `json:"communicationNote"`
		PostmortemScore    *int   `json:"postmortemScore"`
		PostmortemNote     string `json:"postmortemNote"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	for name, v := range map[string]*int{"communicationScore": body.CommunicationScore, "postmortemScore": body.PostmortemScore} {
		if v != nil && (*v < 0 || *v > 10) {
			validationProblem(w, []fieldError{{name, "skor harus 0..10"}})
			return
		}
	}
	assessment := &mentorAssessment{
		ScenarioID:         scenarioID,
		CommunicationScore: body.CommunicationScore,
		CommunicationNote:  strings.TrimSpace(body.CommunicationNote),
		PostmortemScore:    body.PostmortemScore,
		PostmortemNote:     strings.TrimSpace(body.PostmortemNote),
	}
	if err := s.progress.saveAssessment(assessment); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Penilaian gagal disimpan", err.Error())
		return
	}
	actor := actorOf(r)
	s.audit.record(auditEntry{Actor: actor, Action: "assessment.save", Target: scenarioID,
		Detail: fmt.Sprintf("comm=%s pm=%s", scoreStr(body.CommunicationScore), scoreStr(body.PostmortemScore))})
	writeJSON(w, http.StatusOK, assessment)
}

func scoreStr(v *int) string {
	if v == nil {
		return "-"
	}
	return strconv.Itoa(*v)
}

// ------------------------------------------------------------------ ekspor

func (s *server) handleMentorExport(w http.ResponseWriter, r *http.Request) {
	scenarioID := strings.TrimSpace(r.URL.Query().Get("scenario"))
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "markdown"
	}
	if scenarioID == "" {
		writeProblem(w, http.StatusBadRequest, "Param buruk", "scenario wajib")
		return
	}
	sess, err := s.loadLabSession(scenarioID)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "Sesi tidak ditemukan", "jalankan `lab scenario start "+scenarioID+"` dulu")
		return
	}
	assessment, _ := s.progress.getAssessment(scenarioID)
	if assessment == nil {
		assessment = &mentorAssessment{ScenarioID: scenarioID}
	}
	if format == "json" {
		summary := map[string]any{
			"scenarioId": sess.ScenarioID, "traineeName": s.settings.Get().TraineeName,
			"startedAt": unixToRFC3339(sess.StartedAt), "endedAt": optRFC3339(sess.EndedAt),
			"actions": sess.Actions, "answers": sess.Answers, "assessment": assessment,
		}
		writeJSON(w, http.StatusOK, summary)
		return
	}
	if format != "markdown" {
		writeProblem(w, http.StatusBadRequest, "Param buruk", "format harus markdown atau json")
		return
	}
	md := renderMentorReport(sess, s.settings.Get().TraineeName, assessment)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=mentor-report-"+scenarioID+".md")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(md))
}

func renderMentorReport(sess *labSession, trainee string, a *mentorAssessment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Laporan Sesi Mentor — Skenario %s\n\n", sess.ScenarioID)
	fmt.Fprintf(&b, "- Trainee: %s\n", trainee)
	fmt.Fprintf(&b, "- Mulai: %s\n", unixToRFC3339(sess.StartedAt))
	if sess.EndedAt != nil {
		fmt.Fprintf(&b, "- Selesai: %s (durasi %ds)\n", unixToRFC3339(*sess.EndedAt), *sess.EndedAt-sess.StartedAt)
	} else {
		b.WriteString("- Selesai: (masih berjalan)\n")
	}
	if len(sess.Actions) > 0 {
		first := sess.Actions[0].TS
		for _, act := range sess.Actions {
			if act.TS < first {
				first = act.TS
			}
		}
		fmt.Fprintf(&b, "- TTD (aksi pertama): %ds\n", first-sess.StartedAt)
	}
	fmt.Fprintf(&b, "- Aksi tercatat: %d\n", len(sess.Actions))
	fmt.Fprintf(&b, "- Hint dipakai: %d\n", len(sess.HintsUsed))
	fmt.Fprintf(&b, "- Jawaban: %d\n\n", len(sess.Answers))

	b.WriteString("## Timeline aksi\n\n")
	for _, act := range sess.Actions {
		fmt.Fprintf(&b, "- `%s` %s — %s\n", unixToRFC3339(act.TS), act.Cmd, act.Detail)
	}
	if len(sess.Actions) == 0 {
		b.WriteString("(belum ada aksi)\n")
	}
	b.WriteString("\n")

	b.WriteString("## Jawaban\n\n")
	for q, ans := range sess.Answers {
		fmt.Fprintf(&b, "- **%s**: %s\n", q, ans)
	}
	if len(sess.Answers) == 0 {
		b.WriteString("(belum ada jawaban)\n")
	}
	b.WriteString("\n")

	b.WriteString("## Penilaian manual (rubrik /20)\n\n")
	fmt.Fprintf(&b, "- Komunikasi (0-10): %s %s\n", scoreStr(a.CommunicationScore), a.CommunicationNote)
	fmt.Fprintf(&b, "- Postmortem (0-10): %s %s\n", scoreStr(a.PostmortemScore), a.PostmortemNote)
	b.WriteString("\n")
	return b.String()
}
