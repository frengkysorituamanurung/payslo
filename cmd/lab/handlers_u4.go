package main

// Handler HTTP U4: insiden, approvals, runbook, postmortem, pengaturan.
// Mutasi dijaga mutationGuard global; aksi berisiko (approval) tercatat di
// audit lab serve (selain audit PaySLO).

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func writeIncidentError(w http.ResponseWriter, err error, fallback string) {
	code := statusOf(err)
	title := fallback
	if code == http.StatusNotFound {
		title = "Tidak ditemukan"
	}
	writeProblem(w, code, title, err.Error())
}

func (s *server) handleIncidentList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && status != "open" && status != "resolved" && status != "acknowledged" {
		writeProblem(w, http.StatusBadRequest, "Filter buruk", "status harus open|resolved|acknowledged")
		return
	}
	items, err := s.incidents.List(r.Context(), status, q.Get("slo"), q.Get("service"))
	if err != nil {
		writeIncidentError(w, err, "Insiden tak terbaca")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": items, "checkedAt": rfc3339(time.Now())})
}

func (s *server) handleIncidentGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := s.incidents.Get(r.Context(), id)
	if err != nil {
		writeIncidentError(w, err, "Insiden gagal dimuat")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *server) handleIncidentNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Text string `json:"text"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	actor := actorOf(r)
	if err := s.incidents.AddNote(r.Context(), id, actor, body.Text); err != nil {
		writeIncidentError(w, err, "Catatan gagal disimpan")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "incident_id": id})
}

func (s *server) handleIncidentApprovals(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status := r.URL.Query().Get("status")
	if status != "" && status != "pending" && status != "approved" && status != "rejected" && status != "expired" {
		writeProblem(w, http.StatusBadRequest, "Filter buruk", "status harus pending|approved|rejected|expired")
		return
	}
	items, err := s.incidents.ListApprovals(r.Context(), id, status)
	if err != nil {
		writeIncidentError(w, err, "Approval tak terbaca")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": items})
}

func (s *server) handleApprovalsList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && status != "pending" && status != "approved" && status != "rejected" && status != "expired" {
		writeProblem(w, http.StatusBadRequest, "Filter buruk", "status harus pending|approved|rejected|expired")
		return
	}
	items, err := s.incidents.ListApprovals(r.Context(), "", status)
	if err != nil {
		writeIncidentError(w, err, "Approval tak terbaca")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": items})
}

func (s *server) handleApprovalRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Action string `json:"action"`
		Step   int    `json:"step"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	actor := actorOf(r)
	ap, err := s.incidents.RequestApproval(r.Context(), id, strings.TrimSpace(body.Action), body.Step, actor)
	if err != nil {
		if errors.Is(err, errIncidentNotFound) {
			writeProblem(w, http.StatusNotFound, "Tidak ditemukan", err.Error())
			return
		}
		writeIncidentError(w, err, "Permintaan approval gagal")
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "approval.request", Target: ap.ID, Detail: id + " " + ap.Action})
	writeJSON(w, http.StatusCreated, map[string]any{"approval": ap, "deduped": ap.Deduped})
}

func (s *server) handleApprovalDecision(w http.ResponseWriter, r *http.Request) {
	aid := r.PathValue("aid")
	var body struct {
		Decision string  `json:"decision"`
		By       string  `json:"by"`
		Reason   *string `json:"reason"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	actor := actorOf(r)
	by := strings.TrimSpace(body.By)
	reason := ""
	if body.Reason != nil {
		reason = *body.Reason
	}
	ap, err := s.incidents.DecideApproval(r.Context(), aid, strings.TrimSpace(body.Decision), by, strings.TrimSpace(reason))
	if err != nil {
		code := statusOf(err)
		if code == http.StatusConflict || code == http.StatusUnprocessableEntity || code == http.StatusBadRequest {
			if !errors.Is(err, errPaysloDown) {
				s.audit.record(auditEntry{Actor: actor, Action: "approval.decide", Target: aid, Result: "error", Error: err.Error()})
			}
		}
		writeIncidentError(w, err, "Keputusan gagal direkam")
		return
	}
	who := by
	if who == "" {
		who = actor
	}
	s.audit.record(auditEntry{Actor: who, Action: "approval.decide", Target: ap.ID, Detail: ap.IncidentID + " " + ap.Status})
	writeJSON(w, http.StatusOK, map[string]any{"approval": ap})
}

func (s *server) handleRunStep(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil || idx < 0 {
		writeProblem(w, http.StatusBadRequest, "Langkah buruk", "idx harus integer >= 0")
		return
	}
	out, err := s.incidents.RunStep(r.Context(), id, idx)
	if err != nil {
		if errors.Is(err, errApprovalConflict) {
			writeProblem(w, http.StatusConflict, "Butuh approval", err.Error())
			return
		}
		writeIncidentError(w, err, "Langkah gagal dijalankan")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handlePostmortemGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := s.incidents.Get(r.Context(), id)
	if err != nil {
		writeIncidentError(w, err, "Insiden gagal dimuat")
		return
	}
	doc, err := s.postmortems.Get(detail.Incident, id)
	if err != nil {
		writeIncidentError(w, err, "Postmortem tak terbaca")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *server) handlePostmortemPut(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status   string             `json:"status"`
		Sections postmortemSections `json:"sections"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", "body JSON tidak valid: "+err.Error())
		return
	}
	if _, err := s.incidents.Get(r.Context(), id); err != nil {
		writeIncidentError(w, err, "Insiden gagal dimuat")
		return
	}
	actor := actorOf(r)
	doc, err := s.postmortems.Save(id, strings.TrimSpace(body.Status), body.Sections, actor)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Postmortem buruk", err.Error())
		return
	}
	// Render markdown dari header insiden segar.
	detail, derr := s.incidents.Get(r.Context(), id)
	if derr == nil {
		doc.Markdown = renderPostmortem(detail.Incident, doc)
	}
	s.audit.record(auditEntry{Actor: actor, Action: "postmortem.save", Target: id, Detail: doc.Status})
	writeJSON(w, http.StatusOK, doc)
}

func (s *server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settings.Get())
}

func (s *server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TraineeName string `json:"traineeName"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	actor := actorOf(r)
	out, err := s.settings.Set(body.TraineeName)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Pengaturan buruk", err.Error())
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "settings.set", Target: "traineeName", Detail: out.TraineeName})
	writeJSON(w, http.StatusOK, out)
}
