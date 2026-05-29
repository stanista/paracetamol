<p align="center">
  <img src="docs/logo.png" alt="paracetamol" width="180">
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.22-00ADD8">
  <img alt="Docker" src="https://img.shields.io/badge/Docker-watchdog-2496ED">
  <img alt="License" src="https://img.shields.io/badge/license-MIT-green">
</p>

# paracetamol

Tiny Docker container watchdog from another container, based on HTTP checks.

It probes configured URLs on an interval. If a probe fails, or returns a status code outside `ok`, it runs `docker restart` for the configured container.

## Usage

```yaml
services:
  paracetamol:
    image: ghcr.io/stanista/paracetamol:latest
    environment:
      CONFIG: |
        interval: 60
        startup_sleep: 30
        checks:
          app:
            url: http://app/ #address of the app reachable from another container
            ok: [200, 403] 
            restart: app
          app2:
            url: http://app2/
            restart: app2
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    restart: unless-stopped

  app:
    image: nginx:alpine
    container_name: app
    restart: unless-stopped

  app2:
    image: nginx:alpine
    container_name: app2
    restart: unless-stopped
```

`startup_sleep` waits before the first check. This helps when Compose starts everything together and the watched containers need time to become responsive.

## Config

```yaml
interval: 60         # seconds between checks
startup_sleep: 30    # seconds before first check

checks:
  app:
    url: http://app:8080/health
    ok: [200, 204]   # okayish status code
    restart: app
```

## Examples

See:

- `examples/config-file/config.yml`
- `examples/config-file/docker-compose.yml`
- `examples/compose-inline.yml`

## License

MIT
