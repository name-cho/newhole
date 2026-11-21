# Security policy

Newhole is a client for the Loophole tunnel service and talks to the Loophole API and SSH
gateway. Please report vulnerabilities privately through GitHub security advisories instead
of public issues.

## Hardening

* Credentials live in `~/.newhole` with `0600` file permissions.
* TLS verification is enabled by default; `--insecure` only affects the API connection and is
  printed as a warning on startup.
* SSH host keys are not verified by default because the service rotates gateways. Use
  `--host-key-policy known_hosts` together with `NEWHOLE_KNOWN_HOSTS` to enable strict
  verification.
* Client supplied `X-Forwarded-For` and `Forwarded` headers are always scrubbed, so the local
  service sees the real client address.
