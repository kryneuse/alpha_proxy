# Delivery and production deployment

This directory describes how alpha_proxy images are delivered to GHCR and how a
delivered version is deployed to the production server.

There are two separate workflows:

- **Delivery** (`.github/workflows/delivery.yml`) runs automatically after a
  green CI run on `main`. It builds the API and ML images, publishes them to
  GHCR with an immutable commit-SHA tag and a `main` tag, and writes a summary
  with the delivered SHA. It never connects to the server.
- **Deploy production** (`.github/workflows/deploy.yml`) runs only manually. It
  takes a delivered SHA, verifies it and the images, then deploys that version
  to the server over SSH with a smoke test and rollback.

## GitHub environment secrets

The `production` environment must define the following secrets:

| Secret | Purpose |
|---|---|
| `DEPLOY_HOST` | SSH host of the production server |
| `DEPLOY_PORT` | SSH port (usually `22`) |
| `DEPLOY_USER` | SSH user with Docker access and write access to `/opt/alpha-proxy` |
| `DEPLOY_SSH_KEY` | Private SSH key (ed25519) used to connect |
| `DEPLOY_KNOWN_HOSTS` | Contents of the server's `known_hosts` entry |

## Creating the `production` environment

1. In the repository, open **Settings → Environments → New environment**.
2. Name it `production`.
3. Add the secrets listed above.
4. Optionally add a protection rule (required reviewers / wait timer) for extra
   safety. The deploy job runs under this environment.

## Getting and verifying `known_hosts`

Fetch the server's host key fingerprint once, from a trusted machine:

```sh
ssh-keyscan -p <port> <host>
```

Verify the fingerprint against the server's actual key before trusting it, then
store the resulting line in the `DEPLOY_KNOWN_HOSTS` secret. The workflow uses
`StrictHostKeyChecking=yes` with this file and never disables host key checking.

## Requirements for `DEPLOY_USER`

- Member of the `docker` group (or otherwise able to run `docker` and
  `docker compose`).
- Write access to `/opt/alpha-proxy`.
- The server must already contain:
  - `/opt/alpha-proxy/.env` with the runtime configuration;
  - the ML models under `/opt/alpha-proxy/models` (mounted read-only).

## GHCR access for private packages

The server must be authenticated to GHCR once so it can pull private images:

```sh
echo <PAT> | docker login ghcr.io -u <user> --password-stdin
```

Use a classic PAT with **only** the `read:packages` scope. Do **not** store the
token in `/opt/alpha-proxy/.env` or anywhere on disk that the deploy script
reads.

## Delivery

Delivery runs automatically after a successful CI run on `main`. It publishes:

- `ghcr.io/kryneuse/alpha-proxy-api:<sha>` and `...:main`
- `ghcr.io/kryneuse/alpha-proxy-ml:<sha>` and `...:main`

The workflow summary shows the delivered SHA and the exact immutable image
names.

## Manual deployment

Deployment is triggered in the GitHub UI:

**Actions → Deploy production → Run workflow**

Paste the 40-character SHA from the Delivery summary into the `sha` input. The
workflow verifies the SHA, checks that both images exist in GHCR, then deploys
the version to the server.

## Smoke test and rollback

The deploy script backs up the previous `compose.yaml` and `deploy.env` before
applying a new version. If the image pull, `compose up`, or the smoke test
(`/healthz`, `/readyz`) fails, the script restores the previous files and brings
the previous version back up, then exits with a non-zero code. If `deploy.env`
did not exist before, it is removed after rollback so only the existing `.env`
is used. The script never prints `.env` contents or secrets.

## Important: in-memory sessions

The API keeps payload mappings in memory. Recreating the API container removes
the mappings of active `payload_id` values. Run a deployment in a suitable
maintenance window and be aware that in-flight detokenization state is lost on
restart.
