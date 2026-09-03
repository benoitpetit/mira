# Reproducible LoCoMo-style recall benchmark

MIRA includes a deterministic synthetic long-conversation benchmark: 200
sessions, 20 candidate facts per session, and 70% distractor messages. It
measures end-to-end CBA recall latency (p50, p95, p99, average) and MIRA's
memory-management extraction cost. It does not redistribute LoCoMo data and
does not claim answer accuracy or third-party scores.

Every JSON report identifies its benchmark, synthetic dataset, exact retrieval
protocol, token budget, Go runtime, platform, and logical CPU count. This makes
the local measurement auditable without pretending that it is an official
LoCoMo answer-quality run.

Run it with:

```bash
make bench-locomo
```

The report is written to `benchmarks/results/locomo_mira.json`. Configure a
different path with `MIRA_LOCOMO_REPORT=/path/report.json` and the number of
benchmark iterations with `BENCHTIME=50x`.

## Third-party comparisons

Only add numbers that you measured with the same dataset, hardware, token
budget, and protocol. Copy `baselines.example.json`, replace every placeholder
(the example intentionally fails validation), then run:

```bash
MIRA_COMPARE_BASELINES=benchmarks/baselines.local.json make bench-locomo
```

Each baseline must identify its dataset, hardware and protocol, and provide a
positive p95 latency. The baseline file is embedded unchanged in the JSON
report; MIRA does not run, estimate, or fabricate competitor results.

Published competitor figures are intentionally kept out of baseline files.
They are market context, not head-to-head evidence; see
[`docs/MARKET_REFERENCES.md`](../docs/MARKET_REFERENCES.md).
