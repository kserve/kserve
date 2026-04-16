# Future Guides

These guides are parked for future productization phases. The work is
done but they depend on features or integrations not yet available in
the Developer Preview.

| Directory | Description | Blocked on |
|-----------|-------------|------------|
| `2_production_hardening/` | Auth, rate limiting, TLS via Connectivity Link | AI Gateway / API Management not in xKS yet |
| `3_multi_tenant_isolation/` | Cross-namespace adapter management with RBAC | Requires Kuadrant integration, not ready for DP |
| `5_per_adapter_observability/` | Prometheus alerts and Grafana dashboards | vLLM lacks per-adapter metric labels upstream |
| `6_secure_adapter_supply_chain/` | Sigstore signing + ModelScan in storage initializer | Productization-phase concern |
