# Platform Templates

App templates for installing HomelabMon on popular homelab platforms.

## Important: Network Scanning

HomelabMon discovers devices via ARP and mDNS, which requires access to the host network. All templates use `network_mode: host` for this reason. If your platform does not support host networking, you can still run HomelabMon without `--scan` -- it will monitor Docker containers, collect metrics, and sync with peer nodes that handle scanning.

## Platforms

| Platform | Directory | Install method |
|----------|-----------|----------------|
| **Unraid** | `unraid/` | Community Applications XML template |
| **CasaOS** | `casaos/` | App Store docker-compose with `x-casaos` metadata |
| **Cosmos** | `cosmos/` | Cosmos marketplace `cosmos-compose.json` |
| **Synology** | `synology/` | Container Manager project import (docker-compose) |
| **QNAP** | `qnap/` | Container Station application import (docker-compose) |

## Authentication

The web UI is protected by an auto-generated token on first start. To retrieve it:

```bash
# Docker
docker exec homelabmon cat /data/.homelabmon/auth-token

# Native binary
cat ~/.homelabmon/auth-token
```

To disable authentication, add `--no-auth` to the command/args.

## Docker (generic)

```bash
docker run -d --name homelabmon \
  --network host \
  -v ./data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  ghcr.io/dx111ge/homelabmon:latest \
  --ui --scan --bind :9600
```

Or use the `docker-compose.yml` in the repository root.

## Native binary (recommended)

For full network scanning capability, running the native binary is the best option:

```bash
curl -L -o homelabmon https://github.com/dx111ge/homelabmon/releases/latest/download/homelabmon-linux-amd64
chmod +x homelabmon
./homelabmon --ui --scan
```
