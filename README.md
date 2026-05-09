<div align="center">

# lrok

**Tunnel your localhost to a public HTTPS URL — with a reserved subdomain on the free plan.**

[![lrok.io](https://img.shields.io/badge/website-lrok.io-d97706?style=flat-square)](https://lrok.io)
[![Docs](https://img.shields.io/badge/docs-install-d97706?style=flat-square)](https://lrok.io/docs/install)
[![Status](https://img.shields.io/badge/status-public-22c55e?style=flat-square)](https://lrok.io/status)
[![Pricing](https://img.shields.io/badge/pricing-%249%2Fmo-d97706?style=flat-square)](https://lrok.io/pricing)

[lrok.io](https://lrok.io) · [Install](https://lrok.io/docs/install) · [vs ngrok](https://lrok.io/compare/ngrok) · [Changelog](https://lrok.io/changelog) · [Pricing](https://lrok.io/pricing)

</div>

---

## What is lrok?

lrok proxies your local server to a public HTTPS URL so external services (Stripe, GitHub, Auth0, Discord, …) can call it during development. It exists because every other option either rotates your URL on every restart or charges per-tunnel.

```
$ lrok http 3000
  Forwarding https://violet-mole.lrok.io  →  http://127.0.0.1:3000
```

That's it. Real Let's Encrypt cert at the edge, WebSockets pass through unbuffered, request inspector built in.

## Why lrok over the alternatives

|                                | **lrok free**          | ngrok free        | Cloudflare Tunnel | localtunnel |
|--------------------------------|------------------------|-------------------|-------------------|-------------|
| Reserved subdomain             | **1, forever**         | ✗ (Pro only)      | requires domain   | ✗           |
| Custom domain                  | Pro ($9/mo)            | Pro+              | yes               | ✗           |
| Bandwidth metering             | **none**               | hard cap          | none              | none        |
| Concurrent tunnels             | 1 (Pro: unlimited)     | 1                 | unlimited         | 1           |
| Request inspector              | **yes**                | yes               | ✗                 | ✗           |
| Interstitial warning page      | **never**              | yes (free)        | ✗                 | ✗           |
| TCP tunnels                    | Pro                    | Pro+              | spectrum (paid)   | ✗           |
| HTTP basic-auth gating         | **yes (free)**          | Pro               | yes               | ✗           |
| Pricing                        | **free or $9/mo flat**  | $0–$25+/mo tiered | bundled           | free        |

## Install

```sh
# macOS / Linux
curl -fsSL https://lrok.io/install.sh | sh

# Windows (PowerShell)
iwr -useb https://lrok.io/install.ps1 | iex

# Go users
go install github.com/orcs-to/lrok.io-cli/cmd/lrok@latest
```

Both shell installers verify the binary against the published `checksums.txt` before placing it on `$PATH`. Source for both lives in [`scripts/`](./scripts) — read before you pipe.

## Quick start

```sh
# 1. Sign in (browser-based, no copy-paste)
$ lrok login

# 2. Expose http://localhost:3000
$ lrok http 3000
  Forwarding https://violet-mole.lrok.io  →  http://127.0.0.1:3000

# 3. (optional) pin a stable URL — free plan includes 1 reservation
$ lrok reserve stripe-dev
$ lrok http 4242 --hint stripe-dev
  Forwarding https://stripe-dev.lrok.io  →  http://127.0.0.1:4242
```

For headless / SSH sessions: `lrok login --no-browser` — paste a token from [`/dashboard/tokens`](https://lrok.io/dashboard/tokens).

## More

```sh
# Raw TCP — Postgres, Redis, SSH, game servers, anything (Pro plan)
$ lrok tcp 5432
  Forwarding tcp://tcp.lrok.io:30007  →  tcp://127.0.0.1:5432

# Gate the public URL behind HTTP basic auth (free plan)
$ lrok http 3000 --basic-auth user:pass

# Update (verifies SHA-256 from checksums.txt before swap)
$ lrok update
```

## Common workflows

- [Stripe webhooks → localhost](https://lrok.io/use-case/stripe-webhooks)
- [GitHub webhooks](https://lrok.io/use-case/github-webhooks)
- [Discord bot dev](https://lrok.io/use-case/discord-bot)
- [Mobile-app deeplinks](https://lrok.io/use-case/mobile-deeplinks)
- [Expose Next.js](https://lrok.io/use-case/expose-nextjs) · [Django](https://lrok.io/use-case/expose-django) · [Rails](https://lrok.io/use-case/expose-rails) · [Go](https://lrok.io/use-case/expose-go) · 15 more frameworks in the [sitemap](https://lrok.io/sitemap.xml)

## Configuration

`~/.lrok/config` (JSON):

```json
{ "token": "ak_..." }
```

Environment overrides:

| Var | Purpose |
|---|---|
| `LROK_API_URL` | Dashboard / control-plane URL. Default: `https://api.lrok.io`. |
| `LROK_INSECURE=1` | Skip TLS verification on the tunnel control connection. **Local dev only.** |

## Build from source

```sh
go install github.com/orcs-to/lrok.io-cli/cmd/lrok@latest
```

You get a `dev`-tagged binary that refuses to self-replace via `lrok update` so it can't clobber your `$GOPATH/bin`.

## Reporting issues

[github.com/orcs-to/lrok.io-cli/issues](https://github.com/orcs-to/lrok.io-cli/issues) — repro steps and CLI version (`lrok --version`) make debugging an order of magnitude faster.

## License

Release binaries are closed-source. The install scripts under `scripts/` are MIT — read, fork, audit.
