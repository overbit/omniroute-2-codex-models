# OmniRoute → Codex Models

[![CI](https://github.com/overbit/omniroute-2-codex-models/actions/workflows/ci.yml/badge.svg)](https://github.com/overbit/omniroute-2-codex-models/actions/workflows/ci.yml) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**Make OmniRoute models discoverable by Codex CLI.** A tiny, dependency-free Node.js proxy translating `GET /v1/models?prefix=alias` into the Codex `GET /models` catalog. Inference still goes directly to OmniRoute.

## Run with Node.js

Requires Node.js 18+. No npm install.

```sh
git clone https://github.com/overbit/omniroute-2-codex-models.git
cd omniroute-2-codex-models
OMNIROUTE_API_KEY=your-key node proxy.js
curl -fsS http://127.0.0.1:8787/models
```

The key is optional if OmniRoute allows unauthenticated access or Codex supplies a Bearer token.

## Run with Docker

After merging, GitHub Actions publishes a multi-platform image to GHCR:

```sh
docker run -d --name omniroute-2-codex-models --restart unless-stopped \
  -p 127.0.0.1:8787:8787 \
  --add-host host.docker.internal:host-gateway \
  -e OMNIROUTE_API_KEY=your-key \
  ghcr.io/overbit/omniroute-2-codex-models:latest
```

Alternatively build locally: `docker compose up -d --build`. The published package may need its visibility set to **Public** in GitHub package settings.

**Linux:** If OmniRoute only listens on host `127.0.0.1`, use `docker run --network host -e HOST=127.0.0.1 -e OMNIROUTE_MODELS_URL='http://127.0.0.1:20128/v1/models?prefix=alias' -e OMNIROUTE_API_KEY ghcr.io/overbit/omniroute-2-codex-models:latest` instead of the bridged command. No `-p` is needed in host mode.

## Connect to an existing OmniRoute Docker container

If OmniRoute is **already running in Docker**, connect both containers to the same user-defined Docker network. No host-published OmniRoute port is required for model discovery.

**1. Find OmniRoute's container name and network:**

```sh
docker ps --format 'table {{.Names}}\t{{.Networks}}'
docker inspect omniroute --format '{{json .NetworkSettings.Networks}}'
```

Replace `omniroute` with the actual container name. Choose one of its user-defined networks (not the built-in `bridge`). If it has no user-defined network, create one and attach it:

```sh
docker network create omniroute-shared
docker network connect omniroute-shared omniroute
```

The network attachment is safe to omit if the container is already attached to your chosen network.

**2. Start the adapter on that network (Compose):**

```sh
export OMNIROUTE_DOCKER_NETWORK=omniroute-shared  # use your actual network
export OMNIROUTE_MODELS_URL='http://omniroute:20128/v1/models?prefix=alias'
export OMNIROUTE_API_KEY='your-key'               # omit if not needed

docker compose -f compose.yaml -f compose.omniroute.yaml up -d --build
```

The Compose overlay uses an **external** network: it will not create OmniRoute or alter its container. The default hostname is `omniroute`; override `OMNIROUTE_MODELS_URL` if your container or service is named differently. DNS names and aliases must be resolvable on that network.

**Alternatively, run the published image directly:**

```sh
docker run -d --name omniroute-2-codex-models --restart unless-stopped \
  --network omniroute-shared \
  -p 127.0.0.1:8787:8787 \
  -e OMNIROUTE_MODELS_URL='http://omniroute:20128/v1/models?prefix=alias' \
  -e OMNIROUTE_API_KEY='your-key' \
  ghcr.io/overbit/omniroute-2-codex-models:latest
```

**3. Verify:**

```sh
curl -fsS http://127.0.0.1:8787/models
docker logs omniroute-2-codex-models
```

Use OmniRoute's **internal listening port** (20128 in these examples), not an unrelated published host port. OmniRoute must listen on an interface reachable from the Docker network (typically `0.0.0.0` inside its container). If Codex itself runs on the host, keep the Codex inference `base_url` pointed at OmniRoute's **host-accessible published port**; the internal hostname `omniroute` normally resolves only inside Docker.

## Configure Codex

In `~/.codex/config.toml`:

```toml
model_provider = "omniroute"

[model_providers.omniroute]
name = "OmniRoute"
base_url = "http://127.0.0.1:20128/v1"
wire_api = "responses"
env_key = "OMNIROUTE_API_KEY"
model_catalog_url = "http://127.0.0.1:8787/models"
```

The inference `base_url` points to OmniRoute; only `model_catalog_url` points to this adapter. Select an ID returned by `/models`.

## Environment variables

| Variable | Node default | Docker default | Purpose |
| --- | --- | --- | --- |
| `OMNIROUTE_MODELS_URL` | `http://127.0.0.1:20128/v1/models?prefix=alias` | `http://host.docker.internal:20128/v1/models?prefix=alias` (or a Docker service hostname via overlay) | Upstream model catalog |
| `OMNIROUTE_MODELS_API_KEY` | empty | empty | Dedicated upstream key (highest priority) |
| `OMNIROUTE_API_KEY` | empty | empty | Fallback upstream key |
| `HOST` | `127.0.0.1` | `0.0.0.0` | Listener interface |
| `PORT` | `8787` | `8787` | Listener port |

Credentials priority: dedicated models key, incoming Bearer token, fallback key. The adapter is **not a public-facing authenticated service**: keep port 8787 on loopback and never expose it to the Internet.

## Troubleshooting

- **401:** Confirm OmniRoute accepts your key for `/v1/models`.
- **502:** Check upstream URL, networking and JSON response shape.
- **Empty model picker:** Verify `curl -fsS http://127.0.0.1:8787/models` returns a non-empty `models` array and restart Codex.
- **Container cannot reach host:** On Linux, use host networking if OmniRoute binds only to loopback.
- **Container cannot reach OmniRoute container:** Check both containers share a user-defined network, the hostname matches the container or network alias, and OmniRoute listens on its network interface.

## Community

Run `npm test` and `node --check proxy.js`. See [Contributing](CONTRIBUTING.md), [Security](SECURITY.md), and the [MIT license](LICENSE). This is an independent community adapter, not an official OmniRoute or OpenAI project.
