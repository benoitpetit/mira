# Market benchmark references

This page records publicly reported competitor measurements as market context.
They must not be compared numerically with MIRA's local synthetic-recall
benchmark and are never embedded in a MIRA benchmark report.

## Why there is no leaderboard

MIRA's built-in benchmark is deterministic CBA retrieval over a synthetic
4,000-candidate corpus, with no LLM call. Published competitors commonly
measure end-to-end answer quality using a particular hosted model, prompt,
judge, managed service, data split, and retrieval budget. Comparing their
seconds or accuracy percentages directly would be misleading.

## Published references

| System | Source protocol | Published result | Interpretation |
|---|---|---|---|
| MIRA | Local synthetic long-conversation recall; 200 sessions, 4,000 candidates, 2K-token budget, no LLM | Generated locally with `make bench-locomo` | Local retrieval latency only; not an answer-accuracy score. |
| Mem0 managed platform | LoCoMo, single-pass retrieval, top-200 budget | Score 92.5; mean 6,956 tokens; p50 0.88 s | Vendor-managed, end-to-end score. The authors state that managed optimizations are not identical to the open-source SDK. |
| Mem0 managed platform | LongMemEval, same managed stack | Score 94.4; mean 6,800 tokens; p50 1.09 s | Same caveat: not a local SDK or retrieval-only measurement. |
| Zep | LongMemEval, GPT-4o, hosted Zep service | 71.2% score; 2.58 s; 1.6K average context tokens | The paper reports a remote service and GPT-4o answer evaluation, unlike MIRA's local retrieval measurement. |
| Letta | Archived leaderboard harness | No current published LoCoMo/LongMemEval score retained here | The harness requires provider API keys and an LLM judge; code availability is not a measured result. |

Sources: [Mem0 README](https://github.com/mem0ai/mem0/blob/main/README.md),
[Mem0 research page](https://mem0.ai/research),
[Zep LongMemEval study](https://blog.getzep.com/content/files/2025/01/ZEP__USING_KNOWLEDGE_GRAPHS_TO_POWER_LLM_AGENT_MEMORY_2025011700.pdf),
and [Letta's archived leaderboard](https://github.com/letta-ai/letta-leaderboard).

## Fair comparison protocol, if commissioned later

1. Pin each product version and record the exact commit or container digest.
2. Use the same dataset split, answer model, embedding model, prompt, token
   budget, machine, and warm-up policy for every system.
3. Run at least three repetitions and publish raw outputs, percentiles, token
   accounting, failures, and confidence intervals.
4. Report retrieval-only latency separately from ingestion, LLM answer latency,
   and LLM-as-a-judge accuracy.
5. Treat LoCoMo results carefully: its public scoring methodology has been
   disputed, so retain scripts and raw answers for audit.

Until that protocol has been executed, MIRA makes no performance or accuracy
claim against Mem0, Zep, or Letta.
