package main

// Handler HTTP U3: SLO Center (list/detail/save/validate/plan/apply/drift/
// capabilities). Mutasi (PUT/POST) sudah dijaga mutationGuard (Origin +
// X-Lab-Token); save/apply dicatat di audit seperti U2.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *server) handleSLOList(w http.ResponseWriter, r *http.Request) {
	slos, err := s.slos.List(r.Context())
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "SLO tak terbaca", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slos": slos, "checkedAt": rfc3339(time.Now())})
}

func (s *server) handleSLOGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	detail, err := s.slos.Get(r.Context(), name)
	switch {
	case errors.Is(err, errSLONotFound):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "SLO "+name+" tidak ada")
	case isBadSLOName(err):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "nama SLO tidak valid")
	case err != nil:
		writeProblem(w, http.StatusServiceUnavailable, "SLO gagal dimuat", err.Error())
	default:
		writeJSON(w, http.StatusOK, detail)
	}
}

func (s *server) handleSLOSave(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Content string `json:"content"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	actor := actorOf(r)
	info, err := s.slos.Save(r.Context(), name, body.Content)
	switch {
	case isBadSLOName(err):
		s.audit.record(auditEntry{Actor: actor, Action: "slo.save", Target: name, Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadRequest, "Nama SLO ditolak", err.Error())
		return
	case errors.Is(err, errSLONotFound):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "SLO "+name+" tidak ada")
		return
	case err != nil:
		s.audit.record(auditEntry{Actor: actor, Action: "slo.save", Target: name, Result: "error", Error: err.Error()})
		if errors.Is(err, errSLOUnavailable) {
			writeProblem(w, http.StatusServiceUnavailable, "payslo tak tersedia", err.Error())
		} else {
			writeProblem(w, http.StatusBadRequest, "Gagal menyimpan", err.Error())
		}
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "slo.save", Target: info.Name, Detail: info.File})
	writeJSON(w, http.StatusOK, info)
}

func (s *server) handleSLOValidate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Body opsional: {content?} (validasi draf tanpa menyimpan). Body kosong
	// = validasi berkas tersimpan.
	var body struct {
		Content *string `json:"content"`
	}
	if r.ContentLength != 0 {
		dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxSLOBytes+1024))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "Body buruk", "body JSON tidak valid: "+err.Error())
			return
		}
	}
	content, hasContent := "", false
	if body.Content != nil {
		content, hasContent = *body.Content, true
	}
	res, err := s.slos.Validate(r.Context(), name, content, hasContent)
	switch {
	case errors.Is(err, errSLONotFound):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "SLO "+name+" tidak ada")
	case isBadSLOName(err):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "nama SLO tidak valid")
	case err != nil:
		if errors.Is(err, errSLOUnavailable) {
			writeProblem(w, http.StatusServiceUnavailable, "payslo tak tersedia", err.Error())
		} else {
			writeProblem(w, http.StatusBadGateway, "Validasi gagal", err.Error())
		}
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *server) handleSLOPlan(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	plan, err := s.slos.Plan(r.Context(), name)
	switch {
	case isSLOInputError(err):
		writeProblem(w, http.StatusBadRequest, "SLO tidak valid", err.Error())
	case errors.Is(err, errSLONotFound):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "SLO "+name+" tidak ada")
	case isBadSLOName(err):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "nama SLO tidak valid")
	case err != nil:
		writeProblem(w, http.StatusBadGateway, "Plan gagal", err.Error())
	default:
		writeJSON(w, http.StatusOK, plan)
	}
}

func (s *server) handleSLOApply(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Connections []string `json:"connections"`
	}
	if r.ContentLength != 0 {
		dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64*1024))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "Body buruk", "body JSON tidak valid: "+err.Error())
			return
		}
	}
	actor := actorOf(r)
	res, err := s.slos.Apply(r.Context(), name, body.Connections)
	switch {
	case isSLOInputError(err):
		s.audit.record(auditEntry{Actor: actor, Action: "slo.apply", Target: name, Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadRequest, "SLO tidak valid", err.Error())
	case errors.Is(err, errSLONotFound):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "SLO "+name+" tidak ada")
	case isBadSLOName(err):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "nama SLO tidak valid")
	case err != nil:
		s.audit.record(auditEntry{Actor: actor, Action: "slo.apply", Target: name, Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadGateway, "Apply gagal", err.Error())
	default:
		detail := summarizeApply(res)
		s.audit.record(auditEntry{Actor: actor, Action: "slo.apply", Target: name, Detail: detail})
		writeJSON(w, http.StatusOK, res)
	}
}

func (s *server) handleDrift(w http.ResponseWriter, r *http.Request) {
	d, err := s.slos.Drift(r.Context())
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "Drift gagal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	m, err := s.slos.Capabilities(r.Context())
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "Capabilities gagal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// isBadSLOName: penolakan resolver path (traversal/nama buruk) -> 404/400,
// bukan 500. Pesan resolver selalu mengandung kata "ditolak"/"tidak valid".
func isBadSLOName(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "ditolak") || strings.Contains(msg, "tidak valid")
}

func isSLOInputError(err error) bool {
	var ie *inputError
	return errors.As(err, &ie)
}

func summarizeApply(res sloApply) string {
	applied, noop := 0, 0
	for _, r := range res.Results {
		if r.Applied {
			applied++
		}
		if r.NoOp {
			noop++
		}
	}
	return "applied=" + strconv.Itoa(applied) + " noOp=" + strconv.Itoa(noop)
}
