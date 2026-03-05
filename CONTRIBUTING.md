# Contributing to HomelabMon

Thanks for your interest in contributing to HomelabMon! This document covers guidelines for contributing code, reporting bugs, and suggesting features.

## License

HomelabMon is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE). By contributing, you agree that your contributions will be licensed under the same license.

## Getting Started

### Prerequisites

- Go 1.23 or later
- Git

### Building from Source

```bash
git clone https://github.com/dx111ge/homelabmon.git
cd homelabmon
go build -o homelabmon .
./homelabmon version
```

### Running Locally

```bash
# Basic node (metrics collection only)
./homelabmon

# With web dashboard
./homelabmon --ui

# With network scanning
./homelabmon --ui --scan

# With LLM chat (requires Ollama running locally)
./homelabmon --ui --llm http://localhost:11434
```

## How to Contribute

### Reporting Bugs

1. Check [existing issues](https://github.com/dx111ge/homelabmon/issues) first
2. Include your OS, Go version, and homelabmon version (`homelabmon version`)
3. Describe what you expected vs. what happened
4. Include relevant log output if available

### Suggesting Features

Open an issue with the `enhancement` label. Describe your use case and why the feature would be useful for homelab monitoring.

### Submitting Code

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Ensure the project builds: `go build ./...`
5. Ensure vet passes: `go vet ./...`
6. Commit with a clear message describing the change
7. Push to your fork and open a pull request

### Pull Request Guidelines

- Keep PRs focused on a single change
- Follow existing code style and patterns
- Update documentation if your change affects user-facing behavior
- Add yourself to the contributors list if this is your first contribution

## Code Structure

```
homelabmon/
├── cmd/                    # CLI commands (cobra)
├── configs/                # Embedded config files (fingerprints.yaml)
├── internal/
│   ├── agent/              # Collector, heartbeat, host info
│   │   ├── observers/      # System metric observers (CPU, mem, disk, net)
│   │   ├── discovery/      # Service fingerprinting engine
│   │   ├── scanners/       # Network scanners (ARP, mDNS, SNMP)
│   │   └── integrations/   # External API clients (FRITZ!Box, Unifi, etc.)
│   ├── hub/
│   │   ├── api/            # Web UI server + page handlers
│   │   └── llm/            # Ollama LLM client + tool executor
│   ├── mesh/               # Peer-to-peer HTTP transport
│   ├── models/             # Data structs (Host, Metric, Service, etc.)
│   ├── notify/             # Notification senders (ntfy, webhook)
│   ├── plugin/             # Plugin interfaces + registry
│   └── store/              # SQLite store + migrations
├── web/                    # Embedded templates + static files
│   ├── templates/          # Go HTML templates
│   └── static/             # JS, CSS
└── main.go                 # Entry point
```

## Writing Plugins

See [PLUGINS.md](PLUGINS.md) for the full plugin development guide covering:
- **Observers** -- local system metrics
- **Probes** -- service-specific monitoring
- **Scanners** -- passive network discovery
- **Integrations** -- external API data pulls
- **External plugins** -- subprocess-based (any language)

## Design Principles

- **Single binary** -- no external dependencies at runtime
- **Zero config** -- works out of the box, configure only what you need
- **Pure Go** -- no CGO, trivial cross-compilation
- **Homelab-first** -- practical over enterprise, simple over complex
- **Mesh architecture** -- no master node, any node can do anything

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- No unnecessary abstractions -- three similar lines beat a premature helper
- Error messages should be lowercase, no punctuation
- Use `zerolog` for logging (already in the project)
- Prefer stdlib over external dependencies

## Questions?

Open an issue or start a discussion on the repository.
