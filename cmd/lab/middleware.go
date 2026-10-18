package main

// Middleware keamanan lab serve (UI-SPEC §KEAMANAN). Urutan di handler():
// hostAllowlist -> mutationGuard -> nonceCSP -> mux.
// Semua penolakan memakai problem+json dan TANPA header CORS apa pun.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

func contextWithNonce(r *http.Request, nonce string) context.Context {
	return context.WithValue(r.Context(), nonceKey, nonce)
}

// hostAllowlist: hanya 127.0.0.1/localhost/[::1] dengan port serve.
func hostAllowlist(next http.Handler, port string) http.Handler {
	allowed := map[string]bool{
		"127.0.0.1:" + port: true,
		"localhost:" + port: true,
		"[::1]:" + port:     true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[strings.ToLower(r.Host)] {
			writeProblem(w, http.StatusForbidden, "Host ditolak", "lab serve hanya menerima Host loopback")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mutationGuard: POST/PUT/PATCH/DELETE wajib Origin == origin serve DAN
// X-Lab-Token cocok. Melindungi dari situs jahat yang memaksa browser
// memanggil 127.0.0.1 (CSRF/DNS rebinding).
func mutationGuard(next http.Handler, token, port string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			got := r.Header.Get("Origin")
			if got != "http://127.0.0.1:"+port && got != "http://localhost:"+port {
				writeProblem(w, http.StatusForbidden, "Origin ditolak", "mutasi butuh Origin UI lab")
				return
			}
			if r.Header.Get("X-Lab-Token") != token {
				writeProblem(w, http.StatusForbidden, "Token ditolak", "mutasi butuh X-Lab-Token yang valid")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// nonceCSP: nonce per-request (konteks) + header keamanan. CSP sesuai UI-SPEC:
// script inline HANYA via nonce (injeksi token); frame + connect hanya Grafana :3002
// (UI memuat dashboard lewat iframe dan memprobe reachability via fetch no-cors).
func nonceCSP(next http.Handler, _ string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		nonce := hex.EncodeToString(b)
		csp := "default-src 'self'; script-src 'self' 'nonce-" + nonce + "'; " +
			"style-src 'self'; img-src 'self' data:; " +
			"connect-src 'self' http://127.0.0.1:3002; " +
			"frame-src http://127.0.0.1:3002; object-src 'none'; base-uri 'none'"
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r.WithContext(contextWithNonce(r, nonce)))
	})
}
