# CONTRIBUTING — PaySLO Lab

Terima kasih ingin berkontribusi! Lab ini untuk onboarding SRE; kesederhanaan
diutamakan. Baca `AGENTS.MD` sebelum mulai (aturan fase, tech stack, prinsip desain).

## DCO sign-off (wajib)

Semua commit WAJIB memiliki `Signed-off-by` (Developer Certificate of Origin).
Gunakan `git commit -s`:

```sh
git commit -s -m "feat(gateway): tambah validasi amount"
```

Hasilnya di footer commit:

```text
Signed-off-by: Nama Kamu <nama@example.com>
```

PR tanpa sign-off akan ditolak CI (cek `git log --format=%B | grep Signed-off-by`).

## Alur kerja

1. Kerjakan HANYA fase/scope yang diminta (lihat `AGENTS.MD` aturan 1).
2. Satu perubahan = satu commit kecil, pesan Conventional Commits
   (`feat:`, `fix:`, `docs:`, `chore:`, `test:`, ...).
3. Tulis test bersamaan dengan kode; fase tidak selesai jika test belum hijau.
4. Jalankan sebelum push:

```sh
make fmt
make lint
make test
```

5. Jangan menambah dependensi tanpa mencatat di `docs/DECISIONS.md`
   (alasan + perkiraan dampak waktu build).
6. Jangan commit secret/data asli. Semua data sintetis (lihat README peringatan keamanan).

## Melaporkan issue

Sertakan: perintah yang dijalankan, output nyata (bukan parafrase),
versi Docker/Rust/OS, dan langkah reproduksi minimal.
