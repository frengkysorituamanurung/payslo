# PaySLO Lab — workspace Dockerfile (FASE 1).
# cargo-chef (pinned via cargo install) + ONE builder compiling the whole
# workspace once, then one minimal runtime stage per binary.
# Runtimes are `scratch` (no shell by construction) + non-root USER 65532.
# musl static binaries; multi-arch via buildx (TARGETARCH -> musl target).

ARG RUST_VERSION=1.97.1

# ------------------------------------------------------------------ base
FROM rust:${RUST_VERSION}-bookworm AS base
ARG TARGETARCH
ENV CARGO_BUILD_JOBS=2 \
    CARGO_NET_RETRY=10
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends musl-tools ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && rustup target add x86_64-unknown-linux-musl aarch64-unknown-linux-musl \
    && cargo install cargo-chef --version 0.1.78 --locked
# Map Docker arch -> Rust musl target (musl-gcc is arch-native on Debian,
# so per-arch native/emulated builds work without a cross toolchain).
# TARGETARCH is set by buildx; fall back to `uname -m` for classic builds.
RUN ARCH="${TARGETARCH:-}"; \
    if [ -z "$ARCH" ]; then \
      case "$(uname -m)" in aarch64|arm64) ARCH="arm64";; *) ARCH="amd64";; esac; \
    fi; \
    if [ "$ARCH" = "arm64" ]; then echo aarch64-unknown-linux-musl > /musl-target; else echo x86_64-unknown-linux-musl > /musl-target; fi

# ------------------------------------------------------------------ planner
FROM base AS planner
COPY Cargo.toml Cargo.lock rust-toolchain.toml ./
COPY crates ./crates
COPY apps ./apps
COPY payslo ./payslo
RUN cargo chef prepare --recipe-path recipe.json

# ------------------------------------------------------------------ cacher
FROM base AS cacher
COPY --from=planner /app/recipe.json recipe.json
RUN MUSL_TARGET=$(cat /musl-target) && cargo chef cook --release --target "$MUSL_TARGET" --recipe-path recipe.json

# ------------------------------------------------------------------ builder
FROM base AS builder
COPY --from=cacher /app/target /app/target
COPY Cargo.toml Cargo.lock rust-toolchain.toml ./
COPY crates ./crates
COPY apps ./apps
COPY payslo ./payslo
COPY db ./db
RUN MUSL_TARGET=$(cat /musl-target) \
    && cargo build --release --target "$MUSL_TARGET" --workspace \
    && cp "/app/target/$MUSL_TARGET/release/gateway-api" /gateway-api \
    && cp "/app/target/$MUSL_TARGET/release/bank-sim" /bank-sim \
    && cp "/app/target/$MUSL_TARGET/release/merchant-sink" /merchant-sink \
    && cp "/app/target/$MUSL_TARGET/release/datadog-mock" /datadog-mock \
    && cp "/app/target/$MUSL_TARGET/release/payslo" /payslo \
    && echo "$MUSL_TARGET" > /musl-target-built

# ------------------------------------------------- runtime: gateway-api
FROM scratch AS gateway-api
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /gateway-api /usr/local/bin/gateway-api
USER 65532:65532
EXPOSE 3000 3001
ENTRYPOINT ["/usr/local/bin/gateway-api"]

# ------------------------------------------------- runtime: bank-sim
FROM scratch AS bank-sim
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /bank-sim /usr/local/bin/bank-sim
USER 65532:65532
EXPOSE 4000
ENTRYPOINT ["/usr/local/bin/bank-sim"]

# ------------------------------------------------- runtime: merchant-sink
FROM scratch AS merchant-sink
COPY --from=builder /merchant-sink /usr/local/bin/merchant-sink
USER 65532:65532
EXPOSE 5000
ENTRYPOINT ["/usr/local/bin/merchant-sink"]

# ------------------------------------------------- runtime: payslo CLI
FROM scratch AS payslo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /payslo /usr/local/bin/payslo
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/payslo"]

# ------------------------------------------------- runtime: datadog-mock
FROM scratch AS datadog-mock
COPY --from=builder /datadog-mock /usr/local/bin/datadog-mock
USER 65532:65532
EXPOSE 8126
ENTRYPOINT ["/usr/local/bin/datadog-mock"]
