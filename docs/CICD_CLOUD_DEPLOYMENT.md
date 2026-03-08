# CI/CD Deployment to Cloud Server

This guide deploys the shared infrastructure and observability stack (`Prometheus`, `Grafana`, `Jaeger`, `OpenTelemetry Collector`) from this repo to a cloud VM via GitHub Actions.

## 1. Server prerequisites

Install on the target VM:

- Docker Engine
- Docker Compose v2 plugin (`docker compose`)
- Git
- curl

Create deployment directory and clone this repo:

```bash
sudo mkdir -p /opt/padosme/padosme-be-common
sudo chown -R $USER:$USER /opt/padosme
cd /opt/padosme
git clone <your-repo-url> padosme-be-common
```

## 2. Required GitHub Secrets

In this repo, add these Actions secrets:

- `CLOUD_HOST`: VM public IP/DNS
- `CLOUD_PORT`: SSH port (usually `22`)
- `CLOUD_USER`: SSH user
- `CLOUD_SSH_KEY`: private key for SSH auth
- `CLOUD_DEPLOY_PATH`: absolute path to this repo on VM (example: `/opt/padosme/padosme-be-common`)

## 3. Deploy workflow

Workflow file: `.github/workflows/deploy-observability.yml`

Triggers:

- Push to `main`
- Manual trigger (`workflow_dispatch`) with:
  - `deploy_ref` (branch/tag/sha)
  - `strict_auth_check` (`true|false`)

What it does on the VM:

1. Checks out the requested git ref.
2. Runs `docker compose -f docker-compose.yml -f docker-compose.prod.yml pull`.
3. Runs `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --remove-orphans`.
4. Runs observability verification script.

## 4. padosme-auth-service telemetry integration

In `padosme-auth-service` deployment, set:

```env
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=<OBSERVABILITY_VM_PRIVATE_IP>:4317
OTEL_INSECURE=true
OTEL_SERVICE_NAME=padosme-auth-service
OTEL_ENVIRONMENT=production
SERVICE_VERSION=<git-sha-or-version>
```

If both services run in the same Docker network, endpoint can be:

```env
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317
```

## 5. Verification

Script: `scripts/cloud-verify-observability.sh`

Checks:

- Prometheus/Grafana/Jaeger/OTel collector health
- Prometheus targets are UP
- `padosme-auth-service` metrics visible in Prometheus
- `padosme-auth-service` traces visible in Jaeger

Run manually on VM:

```bash
cd /opt/padosme/padosme-be-common
AUTH_SERVICE_NAME=padosme-auth-service STRICT_AUTH_CHECK=true ./scripts/cloud-verify-observability.sh
```

If metrics/traces are missing, generate auth-service traffic and run again.

## 6. Production exposure model

`docker-compose.prod.yml` binds management UIs to loopback (`127.0.0.1`) and keeps OTLP ports open externally:

- Public: `4317`, `4318` (from app services)
- Local only: `3000`, `9090`, `16686`, `15672`, `8000`, `5540`, `8888`, `8889`

Access Grafana/Prometheus/Jaeger via SSH tunnel or reverse proxy.
