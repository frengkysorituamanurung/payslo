// Command labserve: Lab Console server (U0).
//
// API + static UI untuk trainee/mentor di 127.0.0.1:8088. Browser hanya bicara
// ke server ini; server yang memanggil Prometheus/PaySLO/docker (fase
// berikutnya; U0: /session dan /health/stack).
//
// Keamanan (UI-SPEC, diuji di server_test.go):
//   - bind loopback saja (flag --addr ditolak bila host bukan loopback),
//   - Host allowlist (127.0.0.1/localhost/[::1] + port serve),
//   - mutasi wajib Origin == origin serve DAN X-Lab-Token,
//   - tanpa CORS, CSP ketat + nonce untuk injeksi token.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lab serve: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	// Terima `lab serve ...`: buang subcommand "serve" bila ada.
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8088", "alamat listen (wajib loopback)")
	demo := fs.Bool("demo", false, "fixture deterministik, tanpa Docker")
	devProxy := fs.String("dev-proxy", "", "proxy /src/* /@vite/* /node_modules/* ke Vite dev (mis. http://127.0.0.1:5173)")
	webDir := fs.String("web-dir", "", "override disk untuk index.html+aset (dev/test); kosong = pakai embed ui/dist")
	if err := fs.Parse(args); err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("addr tidak valid %q: %w", *addr, err)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("addr %q ditolak: lab serve hanya boleh bind loopback (127.0.0.1/localhost/::1)", *addr)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("gagal membuat token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	srv := newServer(serverConfig{
		demo:      *demo,
		devProxy:  *devProxy,
		webDir:    *webDir,
		token:     token,
		port:      port,
		mentorPIN: os.Getenv("MENTOR_PIN"),
	})
	log.Printf("lab serve di http://%s (demo=%v dev-proxy=%q)", *addr, *demo, *devProxy)
	return http.ListenAndServe(*addr, srv.handler())
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "127.0.0.1" || h == "localhost" || h == "::1" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
