# PaySLO capability matrix

Single source of truth: `payslo_adapter::CAPABILITY_MATRIX`
(`payslo capabilities` prints it). Keep this file in sync when editing.

| feature | prometheus | datadog |
|---|---|---|
| recording rules | yes (PromQL) | no (Datadog evaluates SLI itself) |
| burn-rate alerts | yes (MWMB, any window ≥1m) | yes (error_budget monitors, timeframe 7d/30d/90d only) |
| objective windows | 1m..90d | 7d/30d/90d only (lab windows mapped to 7d + warning) |
| SLI source | metric (PromQL, native) | metric (Datadog queries; PromQL stored opaquely, not evaluated) |
| drift check | yes (file hash) | yes (API content hash) |
| atomic apply | yes (temp + rename + reload) | yes (SLO first, then monitors; retry 429/5xx) |
| runbook link | yes (runbook_url annotation) | yes (message + tag) |

## Explicitly unsupported (warned, never silently dropped)

- Datadog SLO `timeframe` other than 7d/30d/90d: the adapter maps lab
  windows (e.g. 1h) to 7d and prints `warning:` on every plan/apply.
- Datadog cannot evaluate PromQL: SLI `good`/`total` are stored opaquely in
  the mock (and would need Datadog-metric rewrites against real Datadog).
- `slo alert` monitors use `error_budget("<id>").over("<tf>") >= <burn>`
  (field shapes verified against official docs; the mock stores without
  evaluating — see docs/ASSUMPTIONS.md).
