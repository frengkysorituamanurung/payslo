package main

// Handler HTTP U2: payments, bank-status, traffic, routing, release, audit.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// decodeStrict: tolak field tak dikenal (kontrak additionalProperties:false)
// dan body kosong. Balikan pesan jelas.
func decodeStrict(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("body JSON tidak valid: " + err.Error())
	}
	return nil
}

func uuidLike(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// ------------------------------------------------------------------ payments

func (s *server) handlePaymentsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if _, err := demoScenario(r, s.cfg.demo); err != nil {
		writeProblem(w, http.StatusBadRequest, "Param buruk", err.Error())
		return
	}
	filter := paymentFilter{
		Status:   q.Get("status"),
		Provider: q.Get("provider"),
		Channel:  q.Get("channel"),
		Cursor:   q.Get("cursor"),
	}
	if filter.Status != "" && !validStatuses[filter.Status] {
		writeProblem(w, http.StatusBadRequest, "Filter buruk", "status harus CREATED|PENDING|SUCCESS|FAILED|EXPIRED")
		return
	}
	if filter.Provider != "" && !validProviders[filter.Provider] {
		writeProblem(w, http.StatusBadRequest, "Filter buruk", "provider harus bca|mandiri|ewallet")
		return
	}
	if filter.Channel != "" && !validChannels[filter.Channel] {
		writeProblem(w, http.StatusBadRequest, "Filter buruk", "channel harus VA|EWALLET|QRIS")
		return
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLim {
			writeProblem(w, http.StatusBadRequest, "Filter buruk", "limit harus 1..100")
			return
		}
		filter.Limit = n
	}
	page, err := s.payments.List(r.Context(), filter)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Kursor buruk", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *server) handlePaymentCreate(w http.ResponseWriter, r *http.Request) {
	var req createPaymentReq
	if err := decodeStrict(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	if errs := validateCreate(req); len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	actor := actorOf(r)
	target := strings.TrimSpace(req.OrderID)
	sum, err := s.payments.Create(r.Context(), req)
	if err != nil {
		s.audit.record(auditEntry{Actor: actor, Action: "payment.create", Target: target, Result: "error", Error: err.Error()})
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeProblem(w, status, "Gateway gagal", err.Error())
		return
	}
	s.audit.record(auditEntry{
		Actor: actor, Action: "payment.create", Target: sum.ID,
		Detail: target + " " + sum.Channel + " " + strconv.FormatInt(sum.Amount, 10) + " IDR",
	})
	writeJSON(w, http.StatusCreated, sum)
}

func (s *server) handlePaymentDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidLike(id) {
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "id transaksi bukan UUID valid")
		return
	}
	detail, err := s.payments.Get(r.Context(), id)
	switch {
	case errors.Is(err, errPaymentNotFound):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "transaksi tidak ada")
	case err != nil:
		writeProblem(w, http.StatusServiceUnavailable, "Sumber data terganggu", err.Error())
	default:
		writeJSON(w, http.StatusOK, detail)
	}
}

func (s *server) handleBankStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidLike(id) {
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "id transaksi bukan UUID valid")
		return
	}
	bs, err := s.payments.BankStatus(r.Context(), id)
	switch {
	case errors.Is(err, errPaymentNotFound):
		writeProblem(w, http.StatusNotFound, "Tidak ditemukan", "transaksi tidak ada")
	case errors.Is(err, errNoProviderRef):
		writeProblem(w, http.StatusConflict, "Belum ada provider_ref", err.Error())
	case errors.Is(err, errBankDown):
		writeProblem(w, http.StatusBadGateway, "Bank tak terjangkau", err.Error())
	case err != nil:
		writeProblem(w, http.StatusServiceUnavailable, "Sumber data terganggu", err.Error())
	default:
		writeJSON(w, http.StatusOK, bs)
	}
}

// ------------------------------------------------------------------ control

func (s *server) handleTrafficGet(w http.ResponseWriter, r *http.Request) {
	if _, err := demoScenario(r, s.cfg.demo); err != nil {
		writeProblem(w, http.StatusBadRequest, "Param buruk", err.Error())
		return
	}
	st, err := s.control.Traffic(r.Context())
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "Docker tak terjangkau", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *server) handleTrafficPut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RatePerSec json.Number `json:"ratePerSec"`
		Source     *string     `json:"source"`
		CheckedAt  *string     `json:"checkedAt"`
	}
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	rate, err := body.RatePerSec.Int64()
	if err != nil || rate < minTrafficRate || rate > maxTrafficRate {
		validationProblem(w, []fieldError{{"ratePerSec", "ratePerSec harus integer 1..100"}})
		return
	}
	actor := actorOf(r)
	st, err := s.control.SetTraffic(r.Context(), float64(rate))
	if err != nil {
		s.audit.record(auditEntry{Actor: actor, Action: "traffic.set", Target: strconv.FormatInt(rate, 10), Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadGateway, "Docker gagal", err.Error())
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "traffic.set", Target: strconv.FormatInt(rate, 10), Detail: "rate=" + strconv.FormatInt(rate, 10) + "/s"})
	writeJSON(w, http.StatusOK, st)
}

func (s *server) handleRoutingGet(w http.ResponseWriter, r *http.Request) {
	rc, err := s.control.Routing(r.Context())
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "Gateway tak terjangkau", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rc)
}

func (s *server) handleRoutingPut(w http.ResponseWriter, r *http.Request) {
	var rc routingConfig
	if err := decodeStrict(r, &rc); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	if errs := rc.validate(); len(errs) > 0 {
		validationProblem(w, errs)
		return
	}
	actor := actorOf(r)
	out, err := s.control.SetRouting(r.Context(), rc)
	if err != nil {
		s.audit.record(auditEntry{Actor: actor, Action: "routing.set", Target: "gateway", Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadGateway, "Gateway gagal", err.Error())
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "routing.set", Target: "gateway", Detail: routingDetail(out)})
	writeJSON(w, http.StatusOK, out)
}

func routingDetail(rc routingConfig) string {
	part := func(name string, p providerRoute) string {
		return name + "=" + strconv.FormatBool(p.Enabled) + "/w" + strconv.Itoa(p.Weight)
	}
	return part("bca", rc.Bca) + " " + part("mandiri", rc.Mandiri) + " " + part("ewallet", rc.Ewallet)
}

func (s *server) handleReleasePut(w http.ResponseWriter, r *http.Request) {
	var body releaseState
	if err := decodeStrict(r, &body); err != nil {
		writeProblem(w, http.StatusBadRequest, "Body buruk", err.Error())
		return
	}
	if body.Version != "good" && body.Version != "bad" {
		validationProblem(w, []fieldError{{"version", "version harus good atau bad"}})
		return
	}
	actor := actorOf(r)
	out, err := s.control.SetRelease(r.Context(), body.Version)
	if err != nil {
		s.audit.record(auditEntry{Actor: actor, Action: "release.set", Target: "gateway", Result: "error", Error: err.Error()})
		writeProblem(w, http.StatusBadGateway, "Gateway gagal", err.Error())
		return
	}
	s.audit.record(auditEntry{Actor: actor, Action: "release.set", Target: "gateway", Detail: "version=" + out.Version})
	writeJSON(w, http.StatusOK, out)
}

// ------------------------------------------------------------------ audit

func (s *server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeProblem(w, http.StatusBadRequest, "Param buruk", "limit harus 1..200")
			return
		}
		limit = n
	}
	entries := s.audit.list(limit)
	if entries == nil {
		entries = []auditEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

var (
	validStatuses = map[string]bool{"CREATED": true, "PENDING": true, "SUCCESS": true, "FAILED": true, "EXPIRED": true}
	validChannels = map[string]bool{"VA": true, "EWALLET": true, "QRIS": true}
)
