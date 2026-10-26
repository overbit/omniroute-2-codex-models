# OmniRoute → Codex Models

[![CI](https://github.com/overbit/omniroute-2-codex-models/actions/workflows/ci.yml/badge.svg)](https://github.com/overbit/omniroute-2-codex-models/actions/workflows/ci.yml) [![MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**Make OmniRoute models selectable in Codex CLI.** A small, zero-dependency Go adapter translates OmniRoute's OpenAI-style `GET /v1/models?prefix=alias` into Codex's `GET /models` catalog. **Inference still goes directly to OmniRoute.**

- **Low overhead:** static ~5.6 MB Linux binary, around 5–6 MiB idle RSS in a local test; `scratch` Docker image with no language runtime.
- **Fast model picker:** in-memory catalog cache (30 seconds by default), bounded to one 1 MiB response; duplicate concurrent cache misses fetch once.
- **Credential-safe:** caller Bearer tokens go directly upstream and bypass the shared cache. Set `OMNIROUTE_MODELS_API_KEY` to use a shared discovery credential and enable caching for requests coming from Codex.
- **Portable:** runs as a standalone binary or as a multi-architecture Docker image (AMD64/ARM64); supports host-host, Docker-host, and Docker-to-Docker connections.

## Quick start: Docker

Published after merging to `main` by GitHub Actions:

```sh
docker run -d --name omniroute-2-codex-models --restart unless-stopped \
  -p 8787:8787 \
  --env-file ~/.omniroute/.env ghcr.io/overbit/omniroute-2-codex-models:latest
```
Or passing the api key directly
```sh
docker run -d --name omniroute-2-codex-models --restart unless-stopped \
  -p 127.0.0.1:8787:8787 \
  --add-host host.docker.internal:host-gateway \
  -e OMNIROUTE_MODELS_API_KEY='your-key' \
  ghcr.io/overbit/omniroute-2-codex-models:latest
```

Or build locally from the repository:

```sh
docker compose up -d --build
curl -fsS http://127.0.0.1:8787/models
```

The GHCR image must be public (or you must be logged into the registry) to pull it.

On Linux, when OmniRoute runs **on the host** but only listens on `127.0.0.1`, bridge containers cannot reach that listener. Use host networking, e.g.:

```sh
docker run -d --name omniroute-2-codex-models --network host \
  -e HOST=127.0.0.1 \
  -e OMNIROUTE_MODELS_URL='http://127.0.0.1:20128/v1/models?prefix=alias' \
  -e OMNIROUTE_MODELS_API_KEY='your-key' \
  ghcr.io/overbit/omniroute-2-codex-models:latest
```

## Existing OmniRoute Docker container

When OmniRoute already runs in another container, connect the adapter to the **same user-defined Docker network** and target OmniRoute's service/container hostname and **internal** listening port.

```sh
# Choose a network listed under the OmniRoute container.
docker ps --format 'table {{.Names}}\t{{.Networks}}'

# For example, if the existing network is 'omniroute-shared' and the
# existing container is named 'omniroute':
docker run -d --name omniroute-2-codex-models --restart unless-stopped \
  --network omniroute-shared \
  -p 127.0.0.1:8787:8787 \
  -e OMNIROUTE_MODELS_URL='http://omniroute:20128/v1/models?prefix=alias' \
  -e OMNIROUTE_MODELS_API_KEY='your-key' \
  ghcr.io/overbit/omniroute-2-codex-models:latest
```

If OmniRoute is not attached to a user-defined network yet, create one and attach it (`docker network create omniroute-shared && docker network connect omniroute-shared omniroute`). Replace the container name, network, and internal port as needed. OmniRoute must listen on an interface reachable from the network (not only its own `127.0.0.1`).

For Compose, use the optional `compose.omniroute.yaml` overlay:

```sh
export OMNIROUTE_DOCKER_NETWORK=omniroute-shared
export OMNIROUTE_MODELS_URL='http://omniroute:20128/v1/models?prefix=alias'
docker compose -f compose.yaml -f compose.omniroute.yaml up -d --build
```

## Run without Docker

Requires Go 1.23+ to build; no third-party Go modules.

```sh
git clone https://github.com/overbit/omniroute-2-codex-models.git
cd omniroute-2-codex-models
go build -trimpath -o omniroute-2-codex-models .
OMNIROUTE_MODELS_API_KEY='your-key' ./omniroute-2-codex-models
```

By default, the local binary listens at `http://127.0.0.1:8787/models` and queries `http://127.0.0.1:20128/v1/models?prefix=alias`.

## Configure Codex

Add to `~/.codex/config.toml`:

```toml
model_provider = "omniroute"

[model_providers.omniroute]
name = "OmniRoute"
base_url = "http://127.0.0.1:20128/v1"
wire_api = "responses"
env_key = "OMNIROUTE_API_KEY"
model_catalog_url = "http://127.0.0.1:8787/models"
```

Choose a model ID returned by `http://127.0.0.1:8787/models`. `base_url` must be reachable **by Codex** and still points to OmniRoute for inference; only `model_catalog_url` points to this adapter. When Codex runs on the host and OmniRoute runs in Docker, publish OmniRoute's inference port to the host as appropriate. This adapter does not make incompatible models support the Responses API.

## Configuration

| Variable | Local default | Docker default | Purpose |
| --- | --- | --- | --- |
| `OMNIROUTE_MODELS_URL` | `http://127.0.0.1:20128/v1/models?prefix=alias` | `http://host.docker.internal:20128/v1/models?prefix=alias` | Full upstream model listing URL; supports Docker DNS names |
| `OMNIROUTE_MODELS_API_KEY` | unset | unset | Dedicated upstream key (highest priority) |
| `OMNIROUTE_API_KEY` | unset | unset | Fallback key; optional for anonymous upstream |
| `CACHE_TTL_SECONDS` | `30` | `30` | Shared catalog cache TTL (0 disables; max 3600) |
| `HOST` | `127.0.0.1` | `0.0.0.0` | Listener interface |
| `PORT` | `8787` | `8787` | Listener port |

Keys are chosen in order: dedicated models key → incoming Bearer token → fallback key → anonymous. **Bearer-token forwarded requests never use the shared cache.** To get the fast cached path when Codex sends a Bearer token, configure `OMNIROUTE_MODELS_API_KEY` on the **adapter container**. This makes every catalog lookup use the same upstream discovery identity; use forwarding instead if individual users require different model catalogs. The cache holds only the latest successful fixed-credential/anonymous catalog, expires after the configured TTL, and performs a new upstream fetch on expiry. It never serves stale content after a failed refresh.

**Security:** The adapter doesn't authenticate incoming clients. Keep port 8787 bound to localhost, never expose it to the public Internet, and don't put real credentials into published Compose files.

## Performance

On a local Linux test with 40 model entries, an in-memory upstream, ApacheBench (`2200` requests, 16 concurrent connections, three rounds), and Go 1.23:

| Implementation | Idle RSS | Median throughput | p95 latency |
| --- | ---: | ---: | ---: |
| Original Node.js proxy | ~27 MiB | ~3,053 req/s | ~10 ms |
| Go with cache disabled | ~5.7 MiB | ~3,304 req/s | ~11 ms |
| Go with warm 30-second cache | ~5.6 MiB | ~70,943 req/s | ~1 ms |

Numbers are for this mock environment, not real OmniRoute latency or production performance. Caching permits catalog data to be up to 30 seconds old; set `CACHE_TTL_SECONDS=0` for immediate freshness. The adapter is only called for model discovery, not for token streaming or inference.

## Troubleshooting

- **401:** Check upstream authentication and key precedence.
- **502:** Check the upstream hostname/port, JSON format, and logs; the model listing must fit within the 1 MiB limit.
- **No additional models available:** Verify that `curl -fsS http://127.0.0.1:8787/models` returns a nonempty `models` array, then restart Codex.
- **Docker networking:** Ensure both containers share a user-defined network; use the container's internal port, not its host-mapped port.
- **Model list looks stale:** Wait for TTL or set `CACHE_TTL_SECONDS=0`.

## Contributing

Run `go test -race ./...`, `go vet ./...`, and `go test -bench . -benchmem`. See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and [MIT license](LICENSE). Independent community project; not affiliated with OpenAI or OmniRoute.
