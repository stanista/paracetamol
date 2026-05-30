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

It is not a replacement for proper healthchecks, supervision or orchestration. It is a small symptom reliever for containers that are not responding and you just want them to restart.

## Usage

Manual config:

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
            url: http://app/
            restart: app
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    restart: unless-stopped

  app:
    image: nginx:alpine
    container_name: app
    restart: unless-stopped
```

Discovery config:

```yaml
services:
  paracetamol:
    image: ghcr.io/stanista/paracetamol:latest
    environment:
      CONFIG: |
        interval: 60
        startup_sleep: 30
        discover: true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    restart: unless-stopped

  app:
    image: nginx:alpine
    container_name: app
    restart: unless-stopped
    labels:
      - "paracetamol.enable=true"
      - "paracetamol.port=80"
```

## Config

Top-level config:

```yaml
interval: 60
startup_sleep: 30
discover: false

checks:
  app:
    url: http://app:8080/health
    restart: app
    ok: [200, 204]
    failures: 1
    cooldown: 0
```

Fields:

- `interval`: optional, seconds between check rounds, default `60`
- `startup_sleep`: optional, seconds before the first check, default `0`
- `discover`: optional, enable Docker label discovery, default `false`
- `checks`: optional when `discover: true`, otherwise this is where manual checks live

Manual check fields:

- `url`: required, URL reachable from the `paracetamol` container
- `restart`: required, container name or ID to restart
- `ok`: optional, accepted HTTP status codes, default `[200]`
- `failures`: optional, failures before restart, default `1`
- `cooldown`: optional, seconds after restart before another restart, default `0`

## Discovery

Discovery reads Docker labels from running containers. It is opt-in and stricter than manual config.

Required:

- `discover: true` in paracetamol config
- `paracetamol.enable=true` on the target container
- target container must be running
- target container must use Docker restart policy `always` or `unless-stopped`
- target container must provide `paracetamol.url`, `paracetamol.port`, or have exactly one exposed TCP port

Discovery labels:

```yaml
labels:
  - "paracetamol.enable=true"             # required
  - "paracetamol.url=http://app/health"   # optional, full probe URL
  - "paracetamol.port=8080"               # optional, used when url is absent
  - "paracetamol.protocol=http"           # optional, default http
  - "paracetamol.path=/health"            # optional, default /
  - "paracetamol.ok=200,204"              # optional, default 200
  - "paracetamol.failures=3"              # optional, default 3
  - "paracetamol.cooldown=300"            # optional, default 300
```

If `paracetamol.url` is set, `port`, `protocol`, and `path` are ignored.

Manual `checks` override discovered checks with the same name.

## Examples

See:

- `examples/config-file/config.yml`
- `examples/config-file/docker-compose.yml`
- `examples/compose-inline.yml`
- `examples/discovery.yml`

## License

MIT
