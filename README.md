# Newhole

Newhole is a fork of the [Loophole CLI](https://github.com/loophole/cli). It exposes a local
HTTP server, a directory or a WebDAV share to the world through an end to end TLS encrypted
tunnel. The client still speaks the Loophole protocol, so it keeps using the Loophole
gateway and API, but every rough edge of the original client has been reworked.

## What changed compared to upstream

* Reconnects forever with exponential backoff instead of giving up after five attempts, and
  the tunnel survives network outages, laptop suspends and gateway restarts.
* OpenSSH keepalives detect dead connections instead of silently hanging on
  `Initializing secure tunnel... Success!`.
* TLS noise such as `tls: first record does not look like a TLS handshake`,
  `bad record MAC` or `acme/autocert: missing certificate` is explained and logged as
  debug output instead of being printed as an error.
* Authentication is re-established transparently, refresh tokens are actually refreshed and
  a corrupted `tokens.json` produces an actionable message.
* API errors are decoded properly: 4xx/5xx responses, HTML error pages, captive portals and
  timeouts all produce a human readable reason and a hint.
* Custom hostnames are lowercased automatically, which fixes the `--hostname MixedCase`
  breakage.
* The proxy no longer rewrites the `Host` header to `127.0.0.1:<port>`; use
  `--override-host` or `--preserve-host` if you need control over it.
* The real client IP is forwarded in `X-Forwarded-For`/`X-Real-IP` and client supplied
  values are scrubbed, so spoofing is impossible.
* Large query strings and headers work (configurable header limit), request bodies can be
  capped, CORS preflights are handled and an optional per-IP rate limiter is available.
* Unix socket upstreams are supported: `newhole http unix:/run/app.sock`.
* The local directory server hides directory listings unless `--directory-listing` is used.
* The desktop app opens a native WebView2 window on Windows, so Chrome is not required; without
  WebView2 it falls back to Chrome and then to the default browser instead of exiting.
* Deep links inside the desktop UI work after a reload (the local server serves the SPA entry
  point for unknown routes).
* Active desktop tunnels are saved in `active-tunnels.json` with their full remote settings and
  come back after a restart.
* A plain `http://` request to a tunnel is answered with a 301 redirect to `https://`, while
  TLS and h2 keep working on the same listener.
* The desktop app logs to `~/.newhole/logs/desktop-YYYY-MM-DD.log` and keeps working when there
  is no terminal attached.
* The terminal output is serialized, so Windows consoles do not scramble it, and the logger
  no longer kills the process on a broken websocket.
* Credentials are stored in `~/.newhole` with `0600` permissions, the old `~/.loophole`
  directory is migrated automatically.
* `newhole` without arguments starts an interactive wizard that logs in if needed, asks
  what to expose and offers the hostnames used before; `newhole --last` repeats the
  previous tunnel unchanged.
* Used hostnames are remembered in `hostnames.json` and can be listed, filtered and
  forgotten (`newhole account hostnames`, `--forget`, `newhole account unregister`).
* `--daemon` detaches the tunnel (`newhole stop` / `newhole status`), `newhole update`
  replaces the binary with the newest release, `newhole feedback` opens the issue
  tracker and the certificate provisioning is retried instead of failing on the first
  attempt.
* Legacy RSA keys in `~/.newhole/.ssh/id_rsa` are converted to PKCS#8 automatically,
  which is what current OpenSSH versions expect.

## Installation

Build from source:

```bash
go build -tags cli -o newhole .
```

Prebuilt binaries are attached to the releases once the CI runs.

## Quick start

```bash
./newhole account login
./newhole http 3000
./newhole path ./my-directory
./newhole webdav ./my-files
```

## Commands

| Command | Meaning |
| --- | --- |
| `newhole` | interactive wizard (login, what to expose, hostname) |
| `newhole --last` | repeat the previous tunnel |
| `newhole http <port>` | expose a local HTTP server |
| `newhole path <dir>` | expose a directory over HTTP |
| `newhole webdav <dir>` | expose a directory over WebDAV |
| `newhole account login` / `logout` / `whoami` | manage the stored credentials |
| `newhole account hostnames` | list the hostnames used on this machine (`--json`, `--forget`) |
| `newhole account unregister <hostname>` | release a custom hostname |
| `newhole status` / `newhole stop` | inspect / stop the background tunnel |
| `newhole update [--check]` | update the binary from the newest release |
| `newhole feedback` | open the issue tracker (`--print` to only print the URL) |
| `newhole completion <shell>` | print a shell completion script |

## Useful flags

Run `newhole http --help` for the full list. The most relevant ones:

| Flag | Meaning |
| --- | --- |
| `--hostname name` | request a stable hostname (lowercased automatically) |
| `--override-host host` | host header sent to the local service |
| `--preserve-host` | forward the original host header untouched |
| `--basic-auth-username user` / `--basic-auth-password pass` | protect the tunnel with basic auth |
| `--cors https://app.example` | answer CORS preflights (`--cors '*'` allows any origin) |
| `--max-body-bytes n` | reject request bodies larger than `n` bytes |
| `--rate-limit n --rate-limit-window s` | allow `n` requests per client IP per window |
| `--compress` | negotiate gzip with the local service so less data crosses the tunnel |
| `--disable-old-ciphers` | require TLS 1.2 or newer |
| `--insecure` | skip TLS verification of the API (corporate proxies) |
| `--ca-bundle file` | extra root CA used to verify the API |
| `--token value` | use an explicit access token instead of stored credentials |
| `-v`, `--verbose` | debug output, including the classified TLS noise |
| `--daemon` | detach and keep running in the background |
| `--hostname name` | stable hostname, remembered for the next runs |
| `--directory-listing` | (`path` only) show a file listing when there is no `index.html` |
| `--qr` | print a QR code of the public URL |

## Environment variables

| Variable | Meaning |
| --- | --- |
| `NEWHOLE_HOME` | storage directory (default `~/.newhole`), `LOOPHOLE_HOME` is honoured too |
| `NEWHOLE_TOKEN` | access token, useful for CI and notebooks |
| `NEWHOLE_API_ENDPOINT` / `LOOPHOLE_API_ENDPOINT` | override the API endpoint, e.g. `https://api.loophole.cloud:443` |
| `NEWHOLE_GATEWAY_ENDPOINT` | override the SSH gateway, e.g. `gateway.loophole.host:8022` |
| `NEWHOLE_CA_BUNDLE` | extra root CA for the API connection |
| `NEWHOLE_INSECURE_TLS` | `1` to skip API TLS verification |
| `NEWHOLE_ACME_DIRECTORY` | ACME directory used for certificate provisioning |
| `NEWHOLE_HOST_KEY_POLICY` | `insecure` (default) or `known_hosts` |
| `NEWHOLE_KEEPALIVE_INTERVAL` | keepalive interval, default `30s` |
| `NEWHOLE_MAX_RECONNECTS` | `0` (default) means reconnect forever |
| `NEWHOLE_VERBOSE` | `1` to enable debug output |

The `LOOPHOLE_*` variants of these variables are accepted as well, so existing scripts keep
working.

## Development

```bash
make build-cli        # go build -tags cli -o newhole .
make build-desktop    # builds the React bundle first
make test vet fmt
```

Using the tunnel from your own Go program is described in [docs/library.md](docs/library.md)
(including how WebSockets and compression behave), a runnable example lives in
`examples/library`. Every change compared to upstream is listed in [CHANGELOG.md](CHANGELOG.md).

The fork is intentionally self contained: it has no server side dependencies and does not
require any change to the Loophole service.
