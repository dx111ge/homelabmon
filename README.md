# HomelabMon

A single-binary, zero-dependency homelab discovery and monitoring system with mesh networking, auto-discovery, lightweight CMDB, and local LLM integration.

## Features

- **Single binary** -- one Go binary, runs on Linux/macOS/Windows/RPi
- **Pure mesh** -- no master node, any node can serve the dashboard
- **Auto-discovery** -- fingerprints 60+ homelab services (Pi-hole, Plex, Jellyfin, Home Assistant, etc.)
- **Network scanning** -- ARP, mDNS, SNMP discover passive devices (phones, TVs, IoT, printers)
- **Integrations** -- FRITZ!Box, Unifi, Home Assistant, Pi-hole, pfSense API pulls
- **LLM chat** -- local Ollama integration: "What's running on my NAS?"
- **Lightweight CMDB** -- all devices in one SQLite database
- **Notifications** -- ntfy.sh + webhook alerts for host offline / resource thresholds
- **Secure credentials** -- AES-256-GCM encrypted secret store, no passwords in CLI or env
- **External plugins** -- extend with any language via subprocess JSON protocol

## Install

Download the latest binary for your platform from [Releases](https://github.com/dx111ge/homelabmon/releases/latest). No dependencies, no compilation needed.

### Linux (x86_64)

```bash
curl -sL https://github.com/dx111ge/homelabmon/releases/latest/download/homelabmon-linux-amd64 -o homelabmon
chmod +x homelabmon
sudo mv homelabmon /usr/local/bin/
homelabmon --ui
```

### Linux ARM64 (RPi 4/5, Oracle Cloud)

```bash
curl -sL https://github.com/dx111ge/homelabmon/releases/latest/download/homelabmon-linux-arm64 -o homelabmon
chmod +x homelabmon
sudo mv homelabmon /usr/local/bin/
homelabmon --ui
```

### Linux ARM (RPi 3/Zero)

```bash
curl -sL https://github.com/dx111ge/homelabmon/releases/latest/download/homelabmon-linux-arm -o homelabmon
chmod +x homelabmon
sudo mv homelabmon /usr/local/bin/
homelabmon --ui
```

### macOS (Apple Silicon)

```bash
curl -sL https://github.com/dx111ge/homelabmon/releases/latest/download/homelabmon-darwin-arm64 -o homelabmon
chmod +x homelabmon
sudo mv homelabmon /usr/local/bin/
homelabmon --ui
```

### macOS (Intel)

```bash
curl -sL https://github.com/dx111ge/homelabmon/releases/latest/download/homelabmon-darwin-amd64 -o homelabmon
chmod +x homelabmon
sudo mv homelabmon /usr/local/bin/
homelabmon --ui
```

### Windows

Download [homelabmon-windows-amd64.exe](https://github.com/dx111ge/homelabmon/releases/latest/download/homelabmon-windows-amd64.exe) and run:

```powershell
.\homelabmon-windows-amd64.exe --ui
```

### Build from Source

```bash
go build -o homelabmon .
./homelabmon --ui
```

Then open **http://localhost:9600**

## Usage

```bash
homelabmon                                    # bare node: observe + mesh
homelabmon --ui                               # + web dashboard on :9600
homelabmon --scan                             # + network scanning (ARP, mDNS)
homelabmon --llm http://localhost:11434       # + LLM chat (Ollama)
homelabmon --peer 192.168.1.10:9600          # + connect to another node
homelabmon --notify-ntfy https://ntfy.sh/x   # + push notifications
```

All flags can be combined. Every node runs the same binary.

## Three Monitoring Layers

| Layer | Devices | How |
|-------|---------|-----|
| **Agent** | Servers, PCs, NAS, RPi | `homelabmon` runs on the host |
| **Passive** | Phones, tablets, TVs, IoT | ARP/mDNS/SNMP scanning from `--scan` node |
| **Integration** | FRITZ!Box, Unifi, HA, pfSense | API pull via Settings UI |

## Architecture

```
  Node A  <------------>  Node B  <------------>  Node C
  (--ui --llm)            (bare)                  (--ui --scan)
  RPi 4                   NAS                     Mac Mini
```

Each node collects its own metrics and exchanges heartbeats with peers over HTTP. Any node with `--ui` serves the full dashboard showing all nodes.

## Plugin System

Four plugin types, one registry:

| Type | Purpose | Examples |
|------|---------|----------|
| Observer | Local system metrics | CPU, memory, disk, network |
| Probe | Service monitoring | Docker containers |
| Scanner | Network discovery | ARP, mDNS, SNMP |
| Integration | External API pulls | FRITZ!Box, Unifi, HA, Pi-hole, pfSense |

See [PLUGINS.md](PLUGINS.md) for the plugin development guide, including external plugins in any language.

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.23+ |
| Database | SQLite (modernc.org/sqlite, pure Go) |
| Web UI | Go templates + htmx + Alpine.js + Tailwind CSS + Chart.js |
| CLI | cobra + viper |
| Metrics | gopsutil/v4 |
| LLM | Ollama API (local, tool-calling) |

## Cross-Platform Builds

```bash
make all    # builds for linux/amd64, linux/arm64, linux/arm, darwin/amd64, darwin/arm64, windows/amd64
```

## Documentation

- [DESIGN.md](DESIGN.md) -- architecture and design decisions
- [PLUGINS.md](PLUGINS.md) -- plugin development guide
- [PROGRESS.md](PROGRESS.md) -- phase tracker and implementation status
- [CONTRIBUTING.md](CONTRIBUTING.md) -- how to contribute

## License

[GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE)

You are free to use, modify, and distribute this software. If you modify it and make it available over a network, you must share your changes under the same license.
