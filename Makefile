.PHONY: help fmt lint test build deny up up-lite up-prebuilt down reset status open smoke e2e ui-install ui-dev ui-build ui-test ui-e2e ui-audit ui-perf serve serve-demo

help: ## Tampilkan semua target dengan deskripsi
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "%-12s %s\n", $$1, $$2}'

fmt: ## Format kode Rust (cargo fmt --all)
	cargo fmt --all

lint: ## Cek fmt + clippy dengan -D warnings
	cargo fmt --all -- --check
	cargo clippy --workspace --all-targets -- -D warnings

test: ## Jalankan test workspace
	cargo test --workspace

build: ## Build workspace (debug)
	cargo build --workspace

deny: ## Cek lisensi/advisories/ban via cargo-deny
	cargo deny check

up: ## Nyalakan SEMUA profil compose (default+traffic+obs+obs-full+payslo, build lokal)
	docker compose --profile traffic --profile obs --profile obs-full --profile payslo up -d --build

up-lite: ## Nyalakan stack default+traffic+obs+payslo (build lokal)
	docker compose --profile traffic --profile obs --profile payslo up -d --build

up-obs-full: ## Nyalakan obs-full (loki, tempo, alloy) di atas stack berjalan
	docker compose --profile obs-full up -d

up-prebuilt: ## Nyalakan stack dari image GHCR ber-tag versi (PAYSLO_IMAGE_TAG, default v0.1.0)
	docker compose -f docker-compose.yml -f docker-compose.prebuilt.yml --profile traffic --profile obs --profile payslo pull
	docker compose -f docker-compose.yml -f docker-compose.prebuilt.yml --profile traffic --profile obs --profile payslo up -d

down: ## Matikan stack compose (semua profil)
	docker compose --profile traffic --profile obs --profile obs-full --profile payslo down

reset: ## Reset state lab: down -v lalu up-lite (data sintetis dibuat ulang)
	docker compose --profile traffic --profile obs --profile obs-full --profile payslo down -v
	docker compose --profile traffic --profile obs --profile payslo up -d --build

status: ## Tampilkan status service compose
	docker compose --profile traffic --profile obs --profile obs-full --profile payslo ps

open: ## Tampilkan URL lab lokal
	@echo "gateway:  http://127.0.0.1:3000/healthz   admin: http://127.0.0.1:3001 (loopback-only)"
	@echo "envoy:    http://127.0.0.1:8080   admin: http://127.0.0.1:9901"
	@echo "merchant: http://127.0.0.1:5000/state"
	@echo "prom:     http://127.0.0.1:9090   alertmanager: http://127.0.0.1:9093"
	@echo "grafana:  http://127.0.0.1:3002 (admin/lab-only-grafana, anonim=Viewer)"
	@echo "loki:     http://127.0.0.1:3100   tempo: http://127.0.0.1:3200"
	@echo "payslo:   http://127.0.0.1:8081/incidents   ddmock: http://127.0.0.1:8126"
	@echo "ntfy:     http://127.0.0.1:8022/payslo-lab (topic: $${NTFY_TOPIC:-payslo-lab})"

smoke: ## Smoke test end-to-end penuh (scripts/smoke.sh; butuh ~15 mnt)
	bash scripts/smoke.sh

e2e: ## Cek dasar cepat: 1 pembayaran -> callback -> webhook (scripts/e2e-basic.sh)
	bash scripts/e2e-basic.sh

ui-install: ## Install dependensi UI (npm ci, pin eksak di ui/package.json)
	cd ui && npm ci --no-audit --no-fund

ui-dev: ## Vite dev server di 127.0.0.1:5173 (dengan lab serve --dev-proxy untuk API)
	cd ui && npm run dev

ui-build: ## Build UI (tsc + vite -> ui/dist) + salin ke embed Go
	cd ui && npm run build
	rm -rf cmd/lab/web/dist/assets cmd/lab/web/dist/index.html
	cp ui/dist/index.html cmd/lab/web/dist/index.html
	cp -r ui/dist/assets cmd/lab/web/dist/assets

ui-test: ## Gate kualitas UI: tsc + eslint + vitest + prettier-check
	cd ui && npx tsc --noEmit
	cd ui && npx eslint .
	cd ui && npm test -- --run
	cd ui && npx prettier --check .

ui-e2e: ## E2E Playwright vs lab serve --demo (butuh browser chromium)
	bash scripts/ui-e2e.sh

ui-audit: ## Audit UI+Go: npm audit + lisensi + anggaran bundle + gofmt/go vet
	bash scripts/ui-audit.sh

ui-perf: ## Ukur performa UI: anggaran bundle + FCP/DCL (PERF_SOAK_SECONDS untuk soak)
	bash scripts/ui-perf.sh

serve: ## lab serve di 127.0.0.1:8088 (butuh ui-build dulu; API live + UI embed)
	cd cmd/lab && go build -o ../../bin/labserve .
	./bin/labserve serve

serve-demo: ## lab serve --demo (fixture deterministik, tanpa Docker)
	cd cmd/lab && go build -o ../../bin/labserve .
	./bin/labserve serve --demo
