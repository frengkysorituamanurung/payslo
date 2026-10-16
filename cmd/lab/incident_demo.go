package main

// Fixture insiden U4 untuk --demo: satu insiden terbuka bank-bca + satu
// approval remediation + runbook + alert XSS (untuk test pelolosan render).
// Semua mutasi (catatan, approval, postmortem, settings) in-memory.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const demoIncidentID = "inc-9f2c4a7e1b3d5a6c"

func demoIncidentSummary() incidentSummary {
	return incidentSummary{
		ID: demoIncidentID, Title: "payment-success-rate on gateway-api (page)",
		Slo: "payment-success-rate", Service: "gateway-api",
		Status: "open", Severity: "page",
		CreatedAt: "2026-10-16T07:00:00Z",
	}
}

func demoTimeline() []timelineEntry {
	return []timelineEntry{
		{Ts: "2026-10-16T07:00:00Z", Kind: "opened", Actor: "payslo", Message: "incident opened by page alert"},
	}
}

func demoAlerts() []correlatedAlert {
	return []correlatedAlert{
		{
			Fingerprint: "fp-demo-1", Source: "prometheus",
			Slo: "payment-success-rate", Service: "gateway-api",
			Severity: "page", Status: "firing",
			Payload: map[string]any{
				"labels": map[string]any{
					"alertname": "PaySLOBurnRateHigh",
					"slo":       "payment-success-rate",
					"service":   "gateway-api",
					"severity":  "page",
				},
			},
			ReceivedAt: "2026-10-16T07:00:00Z",
		},
		{
			Fingerprint: "fp-demo-xss", Source: "datadog",
			Slo: "payment-success-rate", Service: "gateway-api",
			Severity: "ticket", Status: "firing",
			Payload: map[string]any{
				"labels": map[string]any{
					"alertname": "<script>alert(1)</script>",
					"note":      "<img src=x onerror=alert(2)>",
				},
			},
			ReceivedAt: "2026-10-16T07:01:00Z",
		},
	}
}

func demoRunbookView() runbookView {
	return runbookView{
		Present: true, File: "payment-success-rate.yml",
		Steps: []stepView{
			{Idx: 0, Name: "success-rate per provider", Kind: "diagnostic", Action: "success-rate per provider"},
			{Idx: 1, Name: "profil bank-bca", Kind: "diagnostic", Action: "profil bank-bca"},
			{Idx: 2, Name: "routing gateway saat ini", Kind: "diagnostic", Action: "routing gateway saat ini"},
			{Idx: 3, Name: "catat observasi", Kind: "manual", Action: "catat observasi", Detail: "Buka Provider Health di Grafana."},
			{Idx: 4, Name: "nonaktifkan provider bca", Kind: "remediation", Action: "disable provider bca",
				Impact: "Traffic VA dialihkan dari bca ke mandiri.", RequiresApproval: true},
		},
	}
}

type demoIncidents struct {
	mu        sync.Mutex
	timeline  []timelineEntry
	approvals []approval
	notes     int
	pm        postmortemDoc
	pmSet     bool
	settings  labSettings
	seq       int
	routingB  bool // "bca disabled" (efek remediation demo)
}

func newDemoIncidents() *demoIncidents {
	return &demoIncidents{timeline: demoTimeline()}
}

func (d *demoIncidents) List(_ context.Context, status, slo, service string) ([]incidentSummary, error) {
	in := demoIncidentSummary()
	if status != "" && in.Status != status {
		return []incidentSummary{}, nil
	}
	if slo != "" && in.Slo != slo {
		return []incidentSummary{}, nil
	}
	if service != "" && in.Service != service {
		return []incidentSummary{}, nil
	}
	return []incidentSummary{in}, nil
}

func (d *demoIncidents) Get(_ context.Context, id string) (incidentDetail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id != demoIncidentID {
		return incidentDetail{}, errIncidentNotFound
	}
	rb := demoRunbookView().withApprovals(d.approvals)
	tl := append([]timelineEntry{}, d.timeline...)
	return incidentDetail{
		Incident: demoIncidentSummary(), Timeline: tl, Alerts: demoAlerts(),
		Runbook: rb, Approvals: append([]approval{}, d.approvals...),
	}, nil
}

func (d *demoIncidents) AddNote(_ context.Context, id, actor, text string) error {
	if id != demoIncidentID {
		return errIncidentNotFound
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: teks catatan kosong", errApprovalValidation)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.timeline = append(d.timeline, timelineEntry{
		Ts: rfc3339(time.Now()), Kind: "note", Actor: actor, Message: strings.TrimSpace(text),
	})
	return nil
}

func (d *demoIncidents) ListApprovals(_ context.Context, incident, status string) ([]approval, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := []approval{}
	for _, a := range d.approvals {
		if incident != "" && a.IncidentID != incident {
			continue
		}
		if status != "" && a.Status != status {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (d *demoIncidents) RequestApproval(_ context.Context, incidentID, action string, stepIdx int, by string) (approval, error) {
	if incidentID != demoIncidentID {
		return approval{}, errIncidentNotFound
	}
	step, ok := demoRunbookView().stepForAction(action)
	if !ok || step.Kind != "remediation" {
		return approval{}, fmt.Errorf("%w: aksi %q bukan langkah remediation runbook", errApprovalValidation, action)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, a := range d.approvals {
		if a.Action == action && a.Status == "pending" {
			cpy := a
			cpy.Deduped = true
			return cpy, nil
		}
	}
	d.seq++
	ap := approval{
		ID: fmt.Sprintf("apr-demo-%04d", d.seq), ExecutionID: fmt.Sprintf("exe-demo-%04d", d.seq),
		StepIdx: stepIdx, IncidentID: incidentID, Action: action, Status: "pending",
		RequestedAt: rfc3339(time.Now()),
	}
	d.approvals = append(d.approvals, ap)
	d.timeline = append(d.timeline, timelineEntry{
		Ts: rfc3339(time.Now()), Kind: "approval_requested", Actor: by,
		Message: "approval requested: " + action,
	})
	return ap, nil
}

func (d *demoIncidents) DecideApproval(_ context.Context, approvalID, decision, by, reason string) (approval, error) {
	if decision != "approved" && decision != "rejected" {
		return approval{}, fmt.Errorf("%w: decision must be approved or rejected", errApprovalValidation)
	}
	if by == "" {
		name, ok := d.settingsName()
		if !ok {
			return approval{}, fmt.Errorf("%w: identitas approver wajib (isi pengaturan nama trainee)", errApprovalValidation)
		}
		by = name
	}
	if decision == "rejected" && strings.TrimSpace(reason) == "" {
		return approval{}, fmt.Errorf("%w: reason is required when rejecting", errApprovalValidation)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.approvals {
		if d.approvals[i].ID != approvalID {
			continue
		}
		if d.approvals[i].Status != "pending" {
			return approval{}, fmt.Errorf("%w: approval already decided (%s)", errApprovalConflict, d.approvals[i].Status)
		}
		now := rfc3339(time.Now())
		d.approvals[i].Status = decision
		d.approvals[i].DecidedAt = &now
		d.approvals[i].DecidedBy = &by
		if strings.TrimSpace(reason) != "" {
			r := strings.TrimSpace(reason)
			d.approvals[i].Reason = &r
		}
		d.timeline = append(d.timeline, timelineEntry{
			Ts: now, Kind: "approval_" + decision, Actor: by,
			Message: fmt.Sprintf("approval %s by %s: %s", decision, by, d.approvals[i].Action),
		})
		if decision == "approved" {
			d.routingB = true
			d.timeline = append(d.timeline, timelineEntry{
				Ts: now, Kind: "remediation", Actor: by,
				Message: "remediation executed by " + by + ": " + d.approvals[i].Action + " (bca disabled, demo)",
			})
		}
		return d.approvals[i], nil
	}
	return approval{}, errIncidentNotFound
}

func (d *demoIncidents) settingsName() (string, bool) {
	name := strings.TrimSpace(d.settings.TraineeName)
	return name, name != ""
}

func (d *demoIncidents) GetSettings() labSettings {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.settings
}

func (d *demoIncidents) SetSettings(name string) (labSettings, error) {
	clean, err := validateTraineeName(name)
	if err != nil {
		return labSettings{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.settings = labSettings{TraineeName: clean}
	return d.settings, nil
}

func (d *demoIncidents) GetPM(incident incidentSummary, id string) (postmortemDoc, error) {
	if id != demoIncidentID {
		return postmortemDoc{}, errIncidentNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.pmSet {
		doc := blankPostmortem(id)
		doc.Markdown = renderPostmortem(incident, doc)
		return doc, nil
	}
	doc := d.pm
	doc.Markdown = renderPostmortem(incident, doc)
	return doc, nil
}

func (d *demoIncidents) SavePM(id, status string, sections postmortemSections, by string) (postmortemDoc, error) {
	if id != demoIncidentID {
		return postmortemDoc{}, errIncidentNotFound
	}
	if err := validatePostmortem(status, sections); err != nil {
		return postmortemDoc{}, err
	}
	doc := postmortemDoc{
		IncidentID: id, Status: status, Sections: sections,
		UpdatedAt: rfc3339(time.Now()), UpdatedBy: by, Exists: true,
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pm = doc
	d.pmSet = true
	return doc, nil
}

func (d *demoIncidents) paysloAudit(_ context.Context, _ int) ([]mentorTimelineEntry, error) {
	// Demo: audit PaySLO kosong (pengujian interleave via mergeTimeline murni).
	return []mentorTimelineEntry{}, nil
}

func (d *demoIncidents) RunStep(_ context.Context, incidentID string, idx int) (stepOutput, error) {
	if incidentID != demoIncidentID {
		return stepOutput{}, errIncidentNotFound
	}
	steps := demoRunbookView().Steps
	if idx < 0 || idx >= len(steps) {
		return stepOutput{}, errIncidentNotFound
	}
	st := steps[idx]
	switch st.Kind {
	case "diagnostic":
		return stepOutput{Status: "ok", Detail: "demo output",
			Values: map[string]any{"bca": 0.12, "mandiri": 0.99, "note": "demo"}}, nil
	case "remediation":
		return stepOutput{}, fmt.Errorf("%w: langkah remediation butuh approval PaySLO dulu", errApprovalConflict)
	default:
		return stepOutput{}, fmt.Errorf("%w: langkah manual tak dapat dijalankan", errApprovalValidation)
	}
}
