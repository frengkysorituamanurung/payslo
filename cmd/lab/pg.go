package main

// Postgres access for lab serve (U2).
//
// UI-SPEC: browser hanya bicara ke lab serve; lab serve yang membaca Postgres
// sebagai role `lab_ro` (SELECT saja). Pool dibuat malas (tanpa ping saat start)
// agar server tetap hidup walau DB mati — halaman payments menampilkan
// `dataStatus=source_down`, bukan error.

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Env default (loopback lab). Semua bisa ditimpa untuk test/CI.
const defaultDatabaseURL = "postgres://lab_ro:lab-only-ro-change-me@127.0.0.1:5432/payslo_lab"

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// dbTimeout membatasi setiap query agar UI tidak menggantung.
const dbTimeout = 3 * time.Second

// pgPool menyimpan satu pool per alamat DSN (biasanya satu).
type poolCache struct {
	mu    sync.Mutex
	pools map[string]*pgxpool.Pool
}

func newPoolCache() *poolCache {
	return &poolCache{pools: map[string]*pgxpool.Pool{}}
}

// pool mengembalikan pool untuk dsn, membuatnya bila perlu. Tidak melakukan
// koneksi (pgxpool.NewWithConfig malas) sehingga server start tetap cepat.
func (c *poolCache) pool(dsn string) (*pgxpool.Pool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.pools[dsn]; ok {
		return p, nil
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// Kecil + cepat: cukup untuk UI. Tanpa koneksi minimum agar start instan.
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	c.pools[dsn] = p
	return p, nil
}

// queryCtx memberi batas waktu query.
func queryCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dbTimeout)
}
