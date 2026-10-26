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
| `OMNIROUTE_MODELS_URL` | `http://127.0.0.1:20128/v1/models?prefix=alias` | `http://host.docker.internal:20128/v1/models?prefix=alias` | Upstream model catalog |
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

## Community

Run `npm test` and `node --check proxy.js`. See [Contributing](CONTRIBUTING.md), [Security](SECURITY.md), and the [MIT license](LICENSE). This is an independent community adapter, not an official OmniRoute or OpenAI project.
