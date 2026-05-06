# lrok

> Tunnel your localhost to a public HTTPS URL. Reserved subdomain on the
> free plan. $9/mo for everything else.

[lrok.io](https://lrok.io) · [Docs](https://lrok.io/docs/install) ·
[vs ngrok](https://lrok.io/compare/ngrok) ·
[Changelog](https://lrok.io/changelog)

## Install

```sh
# macOS / Linux
curl -fsSL https://lrok.io/install.sh | sh
```

```powershell
# Windows
iwr -useb https://lrok.io/install.ps1 | iex
```

Both installers verify the binary against the published `checksums.txt`
before placing it on `$PATH`. Source for both scripts lives in
[`scripts/`](./scripts).

## Sign in

```sh
lrok login
```

Opens a browser, you sign in via the lrok dashboard, and the CLI saves
an API key to `~/.lrok/config`. PKCE-style flow — the token never
appears in any URL.

For headless / SSH sessions:

```sh
lrok login --no-browser   # paste token from https://lrok.io/dashboard/tokens
```

## Tunnel

```sh
# Expose http://localhost:3000 to a public HTTPS URL.
$ lrok http 3000
  Forwarding https://violet-mole.lrok.io  ->  http://127.0.0.1:3000

# Pin a stable subdomain (free plan includes 1 reservation).
$ lrok reserve stripe-dev
$ lrok http 4242 --hint stripe-dev
  Forwarding https://stripe-dev.lrok.io  ->  http://127.0.0.1:4242

# TCP tunnel (Pro). Postgres, Redis, SSH, game servers — anything raw.
$ lrok tcp 5432
  Forwarding tcp://tcp.lrok.io:30007  ->  tcp://127.0.0.1:5432

# Gate the public URL behind HTTP basic auth.
$ lrok http 3000 --basic-auth user:pass
```

## Update

```sh
lrok update          # checks GitHub, prompts before replace
lrok update -y       # non-interactive, scripted use
lrok update --check  # status only, no replace
```

Self-update verifies SHA-256 against the release's `checksums.txt`
before swapping the running binary. Linux/macOS atomic rename;
Windows rename-then-move with auto-cleanup of the `.old` file.

## Build from source

```sh
go install github.com/orcs-to/lrok.io-cli/cmd/lrok@latest
```

You'll get a `dev`-tagged binary that refuses to self-replace via
`lrok update` (so it won't clobber your `$GOPATH/bin`).

## Configuration

`~/.lrok/config` (JSON):

```json
{
  "token": "ak_..."
}
```

The CLI reads `LROK_API_URL` for the dashboard endpoint (defaults to
`https://api.lrok.io`) and `LROK_INSECURE=1` to skip TLS verification on
the tunnel control connection (local development only).

## Reporting issues

[github.com/orcs-to/lrok.io-cli/issues](https://github.com/orcs-to/lrok.io-cli/issues)

## License

Closed-source release binaries; the install scripts under `scripts/`
are free to read, fork, and audit.
