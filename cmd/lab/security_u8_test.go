package main

// U8 — uji keamanan tambahan: DNS rebinding (Host asing) + penegakan mutasi.
// Melengkapi TestBadHostRejected/TestMutationRequiresOriginAndToken (U0) dengan
// vektor rebinding eksplisit: nama domain penyerang yang menunjuk ke loopback
// tetap ditolak karena Host bukan salah satu loopback yang diizinkan.

import (
	"net/http"
	"testing"
)

// TestDNSRebindingHostRejected: Host yang BUKAN loopback persis ditolak, walau
// koneksi TCP datang dari 127.0.0.1 (itulah esensi DNS rebinding).
func TestDNSRebindingHostRejected(t *testing.T) {
	s := testServer()
	hosts := []string{
		"evil.example",
		"evil.example:8088",
		"attacker.127.0.0.1.nip.io:8088", // layanan nip.io memetakan ke loopback
		"127.0.0.1.nip.io",
		"localhost.evil.example:8088",
		"127.0.0.1.evil.example:8088",
		"127.0.0.1:8088.evil.example",
		"localhost.",              // trailing dot (FQDN) tidak diizinkan
		"localhost:8089",          // port beda
		"127.0.0.2:8088",          // loopback lain, tetap asing
		"0.0.0.0:8088",            // bind-all bukan loopback
		"[::ffff:127.0.0.1]:8088", // IPv4-mapped, bukan bentuk izin
		"",                        // Host kosong
	}
	for _, host := range hosts {
		for _, target := range []string{"/", "/api/v1/session", "/api/v1/health/stack"} {
			rec := doReq(t, s.handler(), "GET", target, host, goodHeaders())
			if rec.Code != http.StatusForbidden {
				t.Fatalf("rebinding host %q %s: got %d, want 403", host, target, rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("host %q: content-type %q, want problem+json", host, ct)
			}
			for k := range rec.Header() {
				if len(k) >= 14 && k[:14] == "Access-Control" {
					t.Fatalf("host %q: header CORS %q terlarang", host, k)
				}
			}
		}
	}
}

// TestHostAllowlistCaseInsensitive: Host loopback valid apa pun casing-nya
// (HTTP Host tidak case-sensitive) diterima; bentuk lain tetap ditolak.
func TestHostAllowlistCaseInsensitive(t *testing.T) {
	s := testServer()
	allowed := []string{"127.0.0.1:8088", "LOCALHOST:8088", "LocalHost:8088", "[::1]:8088"}
	for _, host := range allowed {
		rec := doReq(t, s.handler(), "GET", "/api/v1/session", host, goodHeaders())
		if rec.Code != http.StatusOK {
			t.Fatalf("host %q: got %d, want 200", host, rec.Code)
		}
	}
}

// TestRebindingCannotMutateEvenWithValidOriginToken: Host asing selalu ditolak
// LEBIH DULU daripada mutationGuard, jadi token/Origin yang "benar" pun tidak
// menembus lewat Host penyerang.
func TestRebindingCannotMutateEvenWithValidOriginToken(t *testing.T) {
	s := testServer()
	headers := map[string]string{
		"Origin":       "http://127.0.0.1:8088",
		"X-Lab-Token":  testToken,
		"Content-Type": "application/json",
	}
	rec := doReq(t, s.handler(), "PUT", "/api/v1/traffic", "evil.example:8088", headers)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rebinding PUT: got %d, want 403", rec.Code)
	}
}

// TestLoopbackDirectMutationRequiresOrigin: dari Host loopback yang sah, mutasi
// tanpa Origin (mis. dipaksa oleh situs jahat via fetch) tetap ditolak CSRF.
func TestLoopbackDirectMutationRequiresOrigin(t *testing.T) {
	s := testServer()
	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"tanpa header sama sekali", map[string]string{}},
		{"hanya token", map[string]string{"X-Lab-Token": testToken}},
		{"Origin null", map[string]string{"Origin": "null", "X-Lab-Token": testToken}},
		{"Origin asing", map[string]string{"Origin": "https://evil.example", "X-Lab-Token": testToken}},
	}
	for _, c := range cases {
		rec := doReq(t, s.handler(), "PUT", "/api/v1/traffic", "127.0.0.1:8088", c.headers)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, want 403", c.name, rec.Code)
		}
	}
}
