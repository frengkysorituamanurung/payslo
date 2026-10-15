package main

// Probe stack live (U0): `docker compose ps --format json`.
// Gagal (Docker mati/stack down) -> labUp=false, services kosong (jujur, U1).
// Bukan kegagalan server: HTTP tetap 200 dengan labUp=false agar UI
// menampilkan onboarding, bukan ErrorState.

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"time"
)

type svcStatus struct {
	name  string
	state string
}

type stackProbe struct {
	labUp    bool
	services []svcStatus
}

type composePsEntry struct {
	Name   string `json:"Name"`
	State  string `json:"State"`
	Health string `json:"Health"`
}

func probeStack() stackProbe {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	var out bytes.Buffer
	cmd.Stdout = &out
	// Timeout keras: UI tidak boleh menggantung karena Docker.
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			return stackProbe{}
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		return stackProbe{}
	}
	var entries []composePsEntry
	// `compose ps --format json`: satu objek JSON per baris (stream).
	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	for {
		var e composePsEntry
		if err := dec.Decode(&e); err != nil {
			break
		}
		if e.Name == "" {
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return stackProbe{}
	}
	probe := stackProbe{labUp: true}
	for _, e := range entries {
		state := "unknown"
		switch e.State {
		case "running":
			state = "up"
			if e.Health != "" && e.Health != "healthy" {
				state = "degraded"
			}
		case "exited", "dead":
			state = "down"
		}
		probe.services = append(probe.services, svcStatus{name: e.Name, state: state})
	}
	return probe
}
