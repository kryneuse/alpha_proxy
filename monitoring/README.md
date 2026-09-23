# Monitoring stack

Prometheus + Grafana for the alpha_proxy service. This stack is separate from
the main `compose.yaml` and attaches to the external Docker network
`alpha-proxy_backend` created by the main stack.

All commands below are run from the repository root.

## Prerequisites

The main alpha_proxy stack must be running first, because this monitoring stack
uses the external network `alpha-proxy_backend`:

```sh
docker compose up -d
```

## Setup

Create the local environment file from the example and set a real Grafana admin
password:

```sh
cp monitoring/.env.example monitoring/.env
# edit monitoring/.env and replace GRAFANA_ADMIN_PASSWORD
```

`monitoring/.env` is git-ignored; never commit it.

## Start / stop

```sh
docker compose -f monitoring/compose.yaml --env-file monitoring/.env up -d
docker compose -f monitoring/compose.yaml --env-file monitoring/.env down
```

## Access

- Grafana: `http://<host>:${GRAFANA_PORT}` (port from `GRAFANA_PORT`)
- Dashboard: **Alpha Proxy**, UID `alpha-proxy-overview`
- Prometheus is not published outside the Docker network.

## Verification

Check Prometheus targets from inside the Prometheus container (its port is not
published on the host):

```sh
docker compose -f monitoring/compose.yaml --env-file monitoring/.env exec prometheus \
  wget -qO- http://127.0.0.1:9090/api/v1/targets
```

Check the provisioned Prometheus datasource health via the Grafana API. The
command loads the credentials from `monitoring/.env` and uses Basic Auth; the
password is never printed:

```sh
set -a; . ./monitoring/.env; set +a
curl --fail --silent --user "${GRAFANA_ADMIN_USER}:${GRAFANA_ADMIN_PASSWORD}" \
  "http://127.0.0.1:${GRAFANA_PORT}/api/datasources/uid/prometheus/health"
```

## Upgrading from Grafana 13.2.1

If Grafana 13.2.1 was started previously, its datasource was not registered
because the Prometheus datasource became a separate plugin in that version and
does not load in a `read_only` container. Before the first start with Grafana
13.1, stop and remove only the Grafana container, then remove the fresh
`grafana_data` volume so the datasource is re-provisioned:

```sh
docker compose -f monitoring/compose.yaml --env-file monitoring/.env stop grafana
docker compose -f monitoring/compose.yaml --env-file monitoring/.env rm -f grafana
docker volume rm alpha-proxy-monitoring_grafana_data
docker compose -f monitoring/compose.yaml --env-file monitoring/.env up -d grafana
```

The Prometheus container and its `prometheus_data` volume are **not** removed —
the volume holds the metrics history.
