# Observability grants

Appended to a role's instructions when the agent is bound with `--tier`. Tier is the
permission unit: it decides which hosts the sandbox lets the agent reach. The know-how is per
service and the same at every tier.

## Shared

- Everything here is read-only. Thanos and Loki accept GET queries only; Grafana is readable
  anonymously. You cannot change alerts, dashboards (unless granted below), or data.
- Prometheus: `bin/promql --host <thanos url> --output json --timeout 60 '<query>'` from the
  main checkout, or `curl "<thanos url>/api/v1/query?query=<urlencoded>"`. Thanos keeps long
  retention; it is the only Prometheus endpoint. `increase()`/`rate()` over ranges longer than
  a few days can inflate on prod; sanity-check against a short window.
- Loki: `logcli` with flags first and the query last, `--since 2h` for relative time (not
  `--from` with a relative string), `--limit`, `-o raw` for JSON lines. `|=` for a literal
  substring; `|~` only for real alternation. `LOKI_ADDR` and `LOKI_ORG_ID` are set for you.
- Grafana read: the anonymous host for the tier; `GET /api/dashboards/uid/<uid>` returns
  provisioned dashboards as JSON. Provisioned dashboard sources live under
  `monitoring/grafana/dashboards/` in the checkout; the JSON there is the truth, Grafana is a
  rendering of it.
- Report numbers with the query that produced them and the time window. Re-run before
  quoting; do not carry a number from an earlier run.
- Logs can contain customer and employee data. Prefer aggregates (`count_over_time`, `sum by`)
  over row dumps; quote only the lines a diagnosis needs; redact emails, names, IDs and tokens
  before they go into a comment. Never query EU prod by any route.

## Tier: dev
- Thanos: `https://thanos.d1-dev-uw2.zipaws.com`
- Loki: `LOKI_ADDR=http://loki.d1-dev-uw2.zipaws.com`
- Grafana (read): `https://grafana.monitoring--grafana.d1-dev-uw2.zipaws.com`
- Job labels differ from app names: e.g. Jobs Service Listings is `job="listings"`
  (interactive) and `job="listings-batch"`.

## Tier: prod
- Thanos: `https://thanos.p1-prod-ue1.zipaws.com`
- Loki: `LOKI_ADDR=http://loki.p1-prod-ue1.zipaws.com`
- Grafana (read): `https://grafana.monitoring--grafana.p1-prod-ue1.zipaws.com`
- This is production traffic. The PII rule above is not optional here. Bursty workloads need
  a same-window-yesterday comparison, never a single sample.

## Grafana writes (dev)
- You may create or update ONE dashboard: the operator's personal preview dashboard,
  UID `ebSsOuMWSIuir1bJWHaNvA`, on `https://grafana.d1.zr.org`. It exists to preview a
  provisioned dashboard change before it is committed. Never create, rename or delete any
  other dashboard or folder, and never change a provisioned dashboard's `uid`.
- Write with `curl -H "cf-access-token: $GRAFANA_DEV_CF_TOKEN" -H 'content-type: application/json'
  -X POST https://grafana.d1.zr.org/api/dashboards/db -d @<file>` where the file is
  `{"dashboard": <json with uid set to the UID above and id null>, "overwrite": true}`.
  To preview a provisioned dashboard: GET its JSON, set `uid` to the preview UID, `id` to
  null, `title` to "<name> (preview)", POST. Report the resulting URL.
- The token acts as the operator. Treat the restriction above as a hard rule even though
  the token itself would allow more.
