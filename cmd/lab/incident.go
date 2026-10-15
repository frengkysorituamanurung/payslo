package main

// Insiden U4: proksi ke payslo-server (sumber kebenaran incident, timeline,
// alert, approvals) + eksekusi runbook di lab serve.
// Browser TIDAK bicara ke payslo-server langsung; lab serve yang meneruskan.
// Tidak ada jalan pintas: remediation hanya dieksekusi setelah decision
// approved tercatat di PaySLO (lihat handleApprovalDecision).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

var (
	errIncidentNotFound   = errors.New("insiden tidak ditemukan")
	errPaysloDown         = errors.New("payslo tak terjangkau")
	errApprovalConflict   = errors.New("approval sudah diputus")
	errApprovalValidation = errors.New("keputusan approval tidak valid")
)

// ------------------------------------------------------------------ DTO

type incidentSummary struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Slo        string  `json:"slo"`
	Service    string  `json:"service"`
	Status     string  `json:"status"`
	Severity   string  `json:"severity"`
	CreatedAt  string  `json:"createdAt"`
	ResolvedAt *string `json:"resolvedAt,omitempty"`
}

type timelineEntry struct {
	Ts      string `json:"ts"`
	Kind    string `json:"kind"`
	Actor   string `json:"actor"`
	Message string `json:"message"`
}

type correlatedAlert struct {
	Fingerprint string         `json:"fingerprint"`
	Source      string         `json:"source"`
	Slo         string         `json:"slo"`
	Service     string         `json:"service"`
	Severity    string         `json:"severity"`
	Status      string         `json:"status"`
	Payload     map[string]any `json:"payload"`
	ReceivedAt  string         `json:"receivedAt"`
}

type approval struct {
	ID          string  `json:"id"`
	ExecutionID string  `json:"executionId"`
	StepIdx     int     `json:"stepIdx"`
	IncidentID  string  `json:"incidentId"`
	Action      string  `json:"action"`
	Status      string  `json:"status"`
	RequestedAt string  `json:"requestedAt"`
	DecidedAt   *string `json:"decidedAt,omitempty"`
	DecidedBy   *string `json:"decidedBy,omitempty"`
	Reason      *string `json:"reason,omitempty"`
	Deduped     bool    `json:"deduped,omitempty"`
}

type incidentDetail struct {
	Incident  incidentSummary   `json:"incident"`
	Timeline  []timelineEntry   `json:"timeline"`
	Alerts    []correlatedAlert `json:"alerts"`
	Runbook   runbookView       `json:"runbook"`
	Approvals []approval        `json:"approvals"`
}

// ------------------------------------------------------------------ interface

type incidents interface {
	List(ctx context.Context, status, slo, service string) ([]incidentSummary, error)
	Get(ctx context.Context, id string) (incidentDetail, error)
	AddNote(ctx context.Context, id, actor, text string) error
	ListApprovals(ctx context.Context, incident, status string) ([]approval, error)
	RequestApproval(ctx context.Context, incidentID, action string, stepIdx int, by string) (approval, error)
	DecideApproval(ctx context.Context, approvalID, decision, by, reason string) (approval, error)
	RunStep(ctx context.Context, incidentID string, idx int) (stepOutput, error)
}

// ------------------------------------------------------------------ klien payslo-server

type paysloClient struct {
	base string
	http *http.Client
}

func newPaysloClient(base string) *paysloClient {
	return &paysloClient{base: strings.TrimSuffix(base, "/"), http: &http.Client{Timeout: 8 * time.Second}}
}

// newLiveIncidents merakit proksi payslo-server + runbook/postmortem/settings.
func newLiveIncidents(cfg serverConfig, workspace string, ctrl control, audit *auditor, pm postmortemSvc, settings settingsSvc) *liveIncidents {
	base := cfg.paysloURL
	if base == "" {
		base = envOr("PAYSLO_URL", "http://127.0.0.1:8081")
	}
	return &liveIncidents{
		payslo:   newPaysloClient(base),
		ctrl:     ctrl,
		runbooks: &runbookStore{dir: filepath.Join(workspace, "runbooks")},
		pm:       pm,
		settings: settings,
		audit:    audit,
	}
}

func (c *paysloClient) get(ctx context.Context, path string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", errPaysloDown, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, nil
}

func (c *paysloClient) post(ctx context.Context, path string, body any) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", errPaysloDown, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, out, nil
}

type statusError struct {
	Status int
	Err    error
	Msg    string
}

func (e *statusError) Error() string { return e.Msg }
func (e *statusError) Unwrap() error { return e.Err }

func paysloErr(status int, body []byte) error {
	var p struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &p)
	msg := strings.TrimSpace(p.Error)
	if msg == "" {
		msg = fmt.Sprintf("payslo HTTP %d", status)
	}
	sentinel := errPaysloDown
	out := http.StatusBadGateway
	switch status {
	case http.StatusNotFound:
		sentinel, out = errIncidentNotFound, http.StatusNotFound
	case http.StatusConflict:
		sentinel, out = errApprovalConflict, http.StatusConflict
	case http.StatusBadRequest:
		sentinel, out = errApprovalValidation, http.StatusBadRequest
	case http.StatusUnprocessableEntity:
		sentinel, out = errApprovalValidation, http.StatusUnprocessableEntity
	}
	return &statusError{Status: out, Err: sentinel, Msg: msg}
}

// statusOf memetakan error ke HTTP: sentinel lokal + status asli PaySLO.
func statusOf(err error) int {
	var se *statusError
	if errors.As(err, &se) {
		return se.Status
	}
	switch {
	case errors.Is(err, errIncidentNotFound):
		return http.StatusNotFound
	case errors.Is(err, errApprovalConflict):
		return http.StatusConflict
	case errors.Is(err, errApprovalValidation):
		return http.StatusBadRequest
	case errors.Is(err, errPaysloDown), errors.Is(err, errSLOUnavailable):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// ------------------------------------------------------------------ live impl

type liveIncidents struct {
	payslo   *paysloClient
	ctrl     control
	runbooks *runbookStore
	pm       postmortemSvc
	settings settingsSvc
	audit    *auditor
}

func (l *liveIncidents) List(ctx context.Context, status, slo, service string) ([]incidentSummary, error) {
	q := url.Values{}
	if slo != "" {
		q.Set("slo", slo)
	}
	if service != "" {
		q.Set("service", service)
	}
	code, body, err := l.payslo.get(ctx, "/incidents?"+q.Encode())
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, paysloErr(code, body)
	}
	var decoded struct {
		Incidents []incidentSummary `json:"incidents"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	out := decoded.Incidents
	if out == nil {
		out = []incidentSummary{}
	}
	if status != "" {
		kept := out[:0]
		for _, in := range out {
			if in.Status == status {
				kept = append(kept, in)
			}
		}
		out = kept
	}
	return out, nil
}

func validIncidentID(id string) bool {
	if !strings.HasPrefix(id, "inc-") || len(id) <= 4 || len(id) > 64 {
		return false
	}
	for _, c := range id[4:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func (l *liveIncidents) Get(ctx context.Context, id string) (incidentDetail, error) {
	if !validIncidentID(id) {
		return incidentDetail{}, errIncidentNotFound
	}
	code, body, err := l.payslo.get(ctx, "/incidents/"+url.PathEscape(id))
	if err != nil {
		return incidentDetail{}, err
	}
	if code != http.StatusOK {
		return incidentDetail{}, paysloErr(code, body)
	}
	var decoded struct {
		Incident incidentSummary   `json:"incident"`
		Timeline []timelineEntry   `json:"timeline"`
		Alerts   []correlatedAlert `json:"alerts"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return incidentDetail{}, err
	}
	d := incidentDetail{
		Incident: decoded.Incident,
		Timeline: orEmptyTimeline(decoded.Timeline),
		Alerts:   orEmptyAlerts(decoded.Alerts),
	}
	// Runbook tereksekusi (milik lab serve) + approvals (milik PaySLO).
	rb := l.runbooks.forSLO(d.Incident.Slo)
	approvals, aerr := l.ListApprovals(ctx, id, "")
	if aerr != nil {
		return incidentDetail{}, aerr
	}
	d.Approvals = approvals
	d.Runbook = rb.withApprovals(approvals)
	return d, nil
}

func orEmptyTimeline(t []timelineEntry) []timelineEntry {
	if t == nil {
		return []timelineEntry{}
	}
	return t
}

func orEmptyAlerts(a []correlatedAlert) []correlatedAlert {
	if a == nil {
		return []correlatedAlert{}
	}
	return a
}

func (l *liveIncidents) AddNote(ctx context.Context, id, actor, text string) error {
	if !validIncidentID(id) {
		return errIncidentNotFound
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: teks catatan kosong", errApprovalValidation)
	}
	if len(text) > 2000 {
		return fmt.Errorf("%w: catatan maks 2000 karakter", errApprovalValidation)
	}
	code, body, err := l.payslo.post(ctx, "/incidents/"+url.PathEscape(id)+"/notes",
		map[string]string{"actor": actor, "text": text})
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return paysloErr(code, body)
	}
	l.audit.record(auditEntry{Actor: actor, Action: "incident.note", Target: id})
	return nil
}

func (l *liveIncidents) ListApprovals(ctx context.Context, incident, status string) ([]approval, error) {
	q := url.Values{}
	if incident != "" {
		q.Set("incident", incident)
	}
	if status != "" {
		q.Set("status", status)
	}
	code, body, err := l.payslo.get(ctx, "/approvals?"+q.Encode())
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, paysloErr(code, body)
	}
	var decoded struct {
		Approvals []approval `json:"approvals"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	if decoded.Approvals == nil {
		return []approval{}, nil
	}
	return decoded.Approvals, nil
}

// RequestApproval: action HARUS cocok dengan langkah remediation di runbook
// (anti aksi bebas), lalu diteruskan ke PaySLO (idempoten di sana).
func (l *liveIncidents) RequestApproval(ctx context.Context, incidentID, action string, stepIdx int, by string) (approval, error) {
	if !validIncidentID(incidentID) {
		return approval{}, errIncidentNotFound
	}
	detail, err := l.Get(ctx, incidentID)
	if err != nil {
		return approval{}, err
	}
	step, ok := detail.Runbook.stepForAction(action)
	if !ok || step.Kind != "remediation" {
		return approval{}, fmt.Errorf("%w: aksi %q bukan langkah remediation runbook", errApprovalValidation, action)
	}
	code, body, err := l.payslo.post(ctx, "/approvals", map[string]any{
		"incident_id": incidentID, "action": action, "step_idx": stepIdx, "requested_by": by,
	})
	if err != nil {
		return approval{}, err
	}
	if code != http.StatusOK {
		return approval{}, paysloErr(code, body)
	}
	var decoded struct {
		Approval approval `json:"approval"`
		Deduped  bool     `json:"deduped"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return approval{}, err
	}
	decoded.Approval.Deduped = decoded.Deduped
	l.audit.record(auditEntry{Actor: by, Action: "approval.request", Target: decoded.Approval.ID, Detail: incidentID + " " + action})
	return decoded.Approval, nil
}

// DecideApproval: teruskan keputusan ke PaySLO (identitas approver wajib).
// Bila approved: eksekusi remediation SETELAH PaySLO mencatat (tak ada jalan
// pintas — tak ada endpoint eksekusi langsung).
func (l *liveIncidents) DecideApproval(ctx context.Context, approvalID, decision, by, reason string) (approval, error) {
	if by == "" {
		if name := strings.TrimSpace(l.settings.Get().TraineeName); name != "" {
			by = name
		} else {
			return approval{}, fmt.Errorf("%w: identitas approver wajib (isi pengaturan nama trainee)", errApprovalValidation)
		}
	}
	code, body, err := l.payslo.post(ctx, "/approvals/"+url.PathEscape(approvalID)+"/decision",
		map[string]string{"decision": decision, "by": by, "reason": reason})
	if err != nil {
		return approval{}, err
	}
	if code != http.StatusOK {
		return approval{}, paysloErr(code, body)
	}
	var decoded struct {
		Approval approval `json:"approval"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return approval{}, err
	}
	ap := decoded.Approval
	l.audit.record(auditEntry{Actor: by, Action: "approval.decide", Target: ap.ID, Detail: ap.IncidentID + " " + decision})
	if decision != "approved" {
		return ap, nil
	}
	// Eksekusi remediation pasca-approval (sinkron; kegagalan dilaporkan
	// jujur + dicatat di timeline, approval tetap approved).
	execErr := l.executeRemediation(ctx, ap, by)
	note := "remediation executed by " + by + ": " + ap.Action
	if execErr != nil {
		note = "remediation FAILED by " + by + ": " + ap.Action + " (" + execErr.Error() + ")"
	}
	_, _, _ = l.payslo.post(ctx, "/incidents/"+url.PathEscape(ap.IncidentID)+"/notes",
		map[string]string{"actor": by, "text": note})
	if execErr != nil {
		return ap, execErr
	}
	return ap, nil
}

// executeRemediation menjalankan langkah lab_routing yang aksinya disetujui.
// Menolak aksi yang tak dikenal runbook (walau approval ada).
func (l *liveIncidents) executeRemediation(ctx context.Context, ap approval, by string) error {
	detail, err := l.Get(ctx, ap.IncidentID)
	if err != nil {
		return err
	}
	step, ok := detail.Runbook.stepForAction(ap.Action)
	if !ok || step.Kind != "remediation" {
		return fmt.Errorf("aksi %q tak ada di runbook (menolak eksekusi)", ap.Action)
	}
	out, err := runLabRouting(ctx, l.ctrl, step.Run)
	if err != nil {
		return err
	}
	_ = by
	_ = out
	return nil
}

// RunStep menjalankan SATU langkah diagnostic (output segar tiap klik).
// Remediation/manual ditolak di sini (409/400): remediation lewat approval.
func (l *liveIncidents) RunStep(ctx context.Context, incidentID string, idx int) (stepOutput, error) {
	detail, err := l.Get(ctx, incidentID)
	if err != nil {
		return stepOutput{}, err
	}
	if idx < 0 || idx >= len(detail.Runbook.Steps) {
		return stepOutput{}, errIncidentNotFound
	}
	step := detail.Runbook.Steps[idx]
	switch step.Kind {
	case "diagnostic":
		return runDiagnostic(ctx, step.Run)
	case "remediation":
		return stepOutput{}, fmt.Errorf("%w: langkah remediation butuh approval PaySLO dulu", errApprovalConflict)
	default:
		return stepOutput{}, fmt.Errorf("%w: langkah manual tak dapat dijalankan", errApprovalValidation)
	}
}
