# Grafana dashboards

**Kserve EPP metrics** dashboards, based on the [llm-d monitoring docs](https://github.com/llm-d/llm-d/blob/main/docs/monitoring/example-promQL-queries.md):

| Dashboard | File | Description |
|-----------|------|-------------|
| **Kserve EPP metrics – All** | `kserve-epp-all-dashboard.json` | **All-in-one:** Routing & load balancing, prefix caching, and P/D disaggregation in a single dashboard |
| **Kserve EPP metrics – Intelligent Routing & Load Balancing** | `routing-load-balancing-dashboard.json` | Request/token distribution, idle GPU time, routing latency |
| **Kserve EPP metrics – Prefix Caching** | `prefix-caching-dashboard.json` | vLLM prefix cache and EPP prefix indexer |
| **Kserve EPP metrics – P/D Disaggregation** | `pd-disaggregation-dashboard.json` | Prefill/decode workers, queue length, P/D decision rates |

Each dashboard has **Reference** text panels and links to the corresponding sections in the llm-d docs. Use the all-in-one dashboard for a single view of all EPP metrics.

---

## 1. Intelligent Routing & Load Balancing

**Panels**

- Request distribution (QPS per instance)
- Token distribution by pod
- Idle GPU time
- Routing decision latency P99

---

## 2. Prefix Caching

**Panels**

- **vLLM prefix cache**
  - Prefix cache hit rate (overall)
  - Per-instance hit rate by pod
  - Cache utilization % by pod/model — uses the metric set by `--kv-cache-usage-percentage-metric` in the LLM inference service config (e.g. `llmisvc_config_default.yaml`). Default: `vllm:kv_cache_usage_perc`.
  - Hits vs queries rate (time series)
- **EPP prefix indexer**
  - EPP prefix indexer size
  - EPP prefix indexer hit ratio (P50, P90)
  - EPP prefix indexer hit bytes (P50, P90)

---

## 3. P/D Disaggregation

**Panels**

- Prefill worker utilization
- Decode worker utilization (KV cache usage; same metric as Prefix Caching — see `--kv-cache-usage-percentage-metric` in LLM inference service config)
- Prefill queue length
- P/D decision rate by type
- Decode-only request rate
- Prefill-decode request rate
- P/D decision ratio (prefill-decode)

---

## Import in Grafana

1. In Grafana: **Dashboards** → **New** → **Import**.
2. **Upload JSON file** and select one of: `kserve-epp-all-dashboard.json`, `routing-load-balancing-dashboard.json`, `prefix-caching-dashboard.json`, `pd-disaggregation-dashboard.json` (or paste its contents).
3. Choose the **Prometheus** datasource that scrapes your cluster (e.g. the one that has the `kserve-lab` ServiceMonitors).
4. Click **Import**.

Repeat for each dashboard you want.

---

## Requirements

- Prometheus scraping vLLM and EPP metrics (see [llm-d Observability and Monitoring](https://github.com/llm-d/llm-d/blob/main/docs/monitoring/README.md)).
- Metric names: dashboards use `vllm:prefix_cache_*`, `vllm:kv_cache_usage_perc`, and other llm-d metrics (vLLM uses the colon form in Prometheus).
- **KV cache usage metric:** The Prefix Caching “Cache utilization %” and P/D Disaggregation “Decode worker utilization” panels use the metric name configured by `--kv-cache-usage-percentage-metric` in the LLM inference service (e.g. `vllm:kv_cache_usage_perc`). If you change that config, update the dashboard queries in `prefix-caching-dashboard.json`, `pd-disaggregation-dashboard.json`, and `kserve-epp-all-dashboard.json` to use the same metric name.
