#!/usr/bin/env bash
# U8 — Build binary `lab serve` (Lab Console, UI React tersemat) untuk 6 target
# platform ke dist/console. Dipanggil oleh .github/workflows/release.yml.
#
# Prasyarat: UI SUDAH dibangun dan disalin ke cmd/lab/web/dist (jalankan
# `make ui-build` lebih dulu). CGO dimatikan agar binary statis (tanpa libc host).
#
# Nama artefak: payslo-lab-console_<tag>_<goos>_<goarch>.tar.gz (zip untuk windows).
set -euo pipefail

tag="${1:?usage: release-console.sh <tag> [outdir]}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${2:-$root/dist/console}"

if [ ! -f "$root/cmd/lab/web/dist/index.html" ]; then
  echo "error: cmd/lab/web/dist/index.html tidak ada — jalankan 'make ui-build' dulu" >&2
  exit 1
fi

rm -rf "$out"
mkdir -p "$out"

targets=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

for t in "${targets[@]}"; do
  goos="${t%%/*}"
  goarch="${t##*/}"
  name="payslo-lab-console_${tag}_${goos}_${goarch}"
  bin="labserve"
  if [ "$goos" = "windows" ]; then
    bin="labserve.exe"
  fi

  echo "==> build ${goos}/${goarch} -> ${name}"
  (
    cd "$root/cmd/lab"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags="-s -w" -o "$out/$name/$bin" .
  )

  if [ "$goos" = "windows" ]; then
    ( cd "$out" && zip -qr "$name.zip" "$name" && rm -rf "$name" )
  else
    ( cd "$out" && tar -czf "$name.tar.gz" "$name" && rm -rf "$name" )
  fi
done

echo "==> artefak di $out"
ls -lh "$out"
