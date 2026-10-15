package main

// Kontrol lab (U2): traffic (loadgen), routing & release gateway.
// Semua mutasi dicatat di audit (aktor, aksi, target, waktu).
//
// Browser TIDAK memanggil gateway/docker langsung; lab serve yang meneruskan
// (UI-SPEC: browser hanya bicara ke lab serve).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ------------------------------------------------------------------ DTO

type providerRoute struct {
	Enabled bool `json:"enabled"`
	Weight  int  `json:"weight"`
}

type routingConfig struct {
	Bca     providerRoute `json:"bca"`
	Mandiri providerRoute `json:"mandiri"`
	Ewallet providerRoute `json:"ewallet"`
}

type releaseState struct {
	Version string `json:"version"`
}

type trafficState struct {
	RatePerSec *float64 `json:"ratePerSec"`
	Source     string   `json:"source"`
	CheckedAt  string   `json:"checkedAt"`
}

const (
	minTrafficRate = 1
	maxTrafficRate = 100
)

func defaultRouting() routingConfig {
	return routingConfig{
		Bca:     providerRoute{Enabled: true, Weight: 60},
		Mandiri: providerRoute{Enabled: true, Weight: 40},
		Ewallet: providerRoute{Enabled: true, Weight: 100},
	}
}

func (r routingConfig) validate() []fieldError {
	var errs []fieldError
	check := func(name string, p providerRoute) {
		if p.Weight < 0 || p.Weight > 100 {
			errs = append(errs, fieldError{"routing." + name + ".weight", "weight harus 0..100"})
		}
	}
	check("bca", r.Bca)
	check("mandiri", r.Mandiri)
	check("ewallet", r.Ewallet)
	return errs
}

// ------------------------------------------------------------------ interface

type control interface {
	Routing(ctx context.Context) (routingConfig, error)
	SetRouting(ctx context.Context, r routingConfig) (routingConfig, error)
	SetRelease(ctx context.Context, v string) (releaseState, error)
	Traffic(ctx context.Context) (trafficState, error)
	SetTraffic(ctx context.Context, rate float64) (trafficState, error)
}

// ------------------------------------------------------------------ live

type liveControl struct {
	adminURL string
	repoRoot string
	http     *http.Client
}

func newLiveControl(adminURL, repoRoot string) *liveControl {
	return &liveControl{adminURL: adminURL, repoRoot: repoRoot, http: &http.Client{Timeout: 8 * time.Second}}
}

func (c *liveControl) Routing(ctx context.Context) (routingConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.adminURL+"/admin/routing", nil)
	if err != nil {
		return routingConfig{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return routingConfig{}, errGatewayDown
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return routingConfig{}, fmt.Errorf("%w (HTTP %d)", errGatewayDown, resp.StatusCode)
	}
	var out routingConfig
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return routingConfig{}, errGatewayDown
	}
	return out, nil
}

func (c *liveControl) SetRouting(ctx context.Context, r routingConfig) (routingConfig, error) {
	body, _ := json.Marshal(r)
	return c.putJSON(ctx, c.adminURL+"/admin/routing", body, func(resp []byte) (routingConfig, error) {
		var out routingConfig
		if err := json.Unmarshal(resp, &out); err != nil {
			return routingConfig{}, errGatewayDown
		}
		return out, nil
	})
}

func (c *liveControl) SetRelease(ctx context.Context, v string) (releaseState, error) {
	body, _ := json.Marshal(releaseState{Version: v})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.adminURL+"/admin/release", bytes.NewReader(body))
	if err != nil {
		return releaseState{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return releaseState{}, errGatewayDown
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseState{}, fmt.Errorf("%w (HTTP %d)", errGatewayDown, resp.StatusCode)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out releaseState
	if err := json.Unmarshal(raw, &out); err != nil {
		return releaseState{}, errGatewayDown
	}
	return out, nil
}

func (c *liveControl) putJSON(ctx context.Context, url string, body []byte, parse func([]byte) (routingConfig, error)) (routingConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return routingConfig{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return routingConfig{}, errGatewayDown
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return routingConfig{}, fmt.Errorf("%w (HTTP %d)", errGatewayDown, resp.StatusCode)
	}
	return parse(raw)
}

func (c *liveControl) Traffic(_ context.Context) (trafficState, error) {
	rate, ok := probeTrafficRate(c.repoRoot)
	if !ok {
		// Jujur: tak terbaca -> null, bukan tebakan (UI-SPEC U1).
		return trafficState{RatePerSec: nil, Source: "unknown", CheckedAt: rfc3339(time.Now())}, nil
	}
	return trafficState{RatePerSec: &rate, Source: "loadgen", CheckedAt: rfc3339(time.Now())}, nil
}

func (c *liveControl) SetTraffic(_ context.Context, rate float64) (trafficState, error) {
	rateStr := strconv.FormatInt(int64(rate), 10)
	cmd := exec.Command("docker", "compose", "--profile", "traffic", "up", "-d", "loadgen")
	cmd.Dir = c.repoRoot
	cmd.Env = append(cmd.Environ(), "RATE_PER_SEC="+rateStr)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return trafficState{}, fmt.Errorf("docker compose gagal: %s", strings.TrimSpace(lastLine(out.String())))
	}
	return trafficState{RatePerSec: &rate, Source: "loadgen", CheckedAt: rfc3339(time.Now())}, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) == 0 {
		return s
	}
	return lines[len(lines)-1]
}

// probeTrafficRate membaca RATE_PER_SEC dari env kontainer loadgen
// (docker compose ps -q + docker inspect). ok=false bila tak terbaca.
func probeTrafficRate(repoRoot string) (float64, bool) {
	id, ok := composeContainerID(repoRoot, "loadgen")
	if !ok {
		return 0, false
	}
	cmd := exec.Command("docker", "inspect", id, "--format", "{{range .Config.Env}}{{println .}}{{end}}")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if v, found := strings.CutPrefix(line, "RATE_PER_SEC="); found {
			n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

func composeContainerID(repoRoot, service string) (string, bool) {
	cmd := exec.Command("docker", "compose", "--profile", "traffic", "ps", "-q", service)
	cmd.Dir = repoRoot
	done := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		o, e := cmd.Output()
		done <- struct {
			out []byte
			err error
		}{o, e}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return "", false
		}
		id := strings.TrimSpace(string(r.out))
		if id == "" {
			return "", false
		}
		return strings.Split(id, "\n")[0], true
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		return "", false
	}
}

// ------------------------------------------------------------------ demo

type demoControl struct {
	mu      sync.Mutex
	routing routingConfig
	release string
	traffic float64
}

func newDemoControl() *demoControl {
	return &demoControl{routing: defaultRouting(), release: "good", traffic: 2}
}

func (d *demoControl) Routing(_ context.Context) (routingConfig, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.routing, nil
}

func (d *demoControl) SetRouting(_ context.Context, r routingConfig) (routingConfig, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.routing = r
	return r, nil
}

func (d *demoControl) SetRelease(_ context.Context, v string) (releaseState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.release = v
	return releaseState{Version: v}, nil
}

func (d *demoControl) Traffic(_ context.Context) (trafficState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rate := d.traffic
	return trafficState{RatePerSec: &rate, Source: "loadgen", CheckedAt: demoBase}, nil
}

func (d *demoControl) SetTraffic(_ context.Context, rate float64) (trafficState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.traffic = rate
	return trafficState{RatePerSec: &rate, Source: "loadgen", CheckedAt: demoBase}, nil
}

// ------------------------------------------------------------------ helpers

func actorOf(r *http.Request) string {
	if a := strings.TrimSpace(r.Header.Get("X-Lab-Actor")); a != "" {
		if len(a) > 64 {
			return a[:64]
		}
		return a
	}
	return "lab-console"
}

func validationProblem(w http.ResponseWriter, errs []fieldError) {
	detail := "masukan tidak valid"
	if len(errs) > 0 {
		detail = errs[0].Message
	}
	writeProblemFields(w, http.StatusBadRequest, "Validasi gagal", detail, errs)
}
