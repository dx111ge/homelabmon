# Plugin Development Guide

This guide covers how to write plugins for HomelabMon. The same plugin architecture is shared with [BYKT/Aria](../aria/) ESM, so plugins can be reused across both systems.

## Plugin Types

HomelabMon has four plugin types. Each serves a different purpose:

| Type | Purpose | Runs on | Examples |
|------|---------|---------|---------|
| **Observer** | Collect local system metrics | Every node | CPU, memory, disk, network, processes, services |
| **Probe** | Monitor a specific service | Nodes where service is detected | Docker, PostgreSQL, Redis, nginx, certificates |
| **Scanner** | Discover passive network devices | Nodes with `--scan` | ARP, mDNS, SNMP, ping |
| **Integration** | Pull data from external APIs | Configured nodes | Unifi, Home Assistant, pfSense, Pi-hole |

## Base Plugin Interface

Every plugin implements the base interface:

```go
package plugin

type PluginType string

const (
    TypeObserver    PluginType = "observer"
    TypeProbe       PluginType = "probe"
    TypeScanner     PluginType = "scanner"
    TypeIntegration PluginType = "integration"
)

type Plugin interface {
    // Name returns a unique identifier, e.g. "cpu", "docker", "arp-scanner"
    Name() string

    // Type returns the plugin category
    Type() PluginType

    // Detect checks if this plugin can run on the current system.
    // Called once at startup. Return false to skip activation.
    // Examples: Docker probe returns false if Docker daemon isn't running.
    //           ARP scanner returns false if --scan is not enabled.
    Detect() bool

    // Interval returns how often Collect/Scan/Sync should be called.
    Interval() time.Duration

    // Start is called when the plugin is activated. Use for setup.
    Start(ctx context.Context) error

    // Stop is called on shutdown. Clean up resources.
    Stop() error
}
```

## Writing an Observer

Observers collect local system metrics. They return partial results that the Collector merges into a `MetricSnapshot`.

```go
package observers

import (
    "context"
    "time"

    "github.com/dx111ge/homelabmon/internal/plugin"
)

type TemperatureObserver struct{}

// Compile-time check: ensure we implement the Observer interface
var _ plugin.Observer = (*TemperatureObserver)(nil)

func (o *TemperatureObserver) Name() string           { return "temperature" }
func (o *TemperatureObserver) Type() plugin.PluginType { return plugin.TypeObserver }
func (o *TemperatureObserver) Interval() time.Duration { return 30 * time.Second }
func (o *TemperatureObserver) Start(ctx context.Context) error { return nil }
func (o *TemperatureObserver) Stop() error                     { return nil }

func (o *TemperatureObserver) Detect() bool {
    // Check if temperature sensors are available
    // On Linux: /sys/class/thermal/thermal_zone*/temp
    // On RPi: vcgencmd measure_temp
    // On Windows: WMI MSAcpi_ThermalZoneTemperature
    // On macOS: powermetrics or SMC
    return runtime.GOOS == "linux" // example: only Linux for now
}

func (o *TemperatureObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
    // Read temperature from system
    temp := readCPUTemp() // your implementation

    return &plugin.ObserverResult{
        Extra: map[string]interface{}{
            "cpu_temp": temp,
        },
    }, nil
}
```

### Observer Result

Observers return an `ObserverResult` with typed fields for standard metrics and an `Extra` map for custom data:

```go
type ObserverResult struct {
    // Standard metrics (merged into MetricSnapshot)
    CPUPercent     *float64
    Load1          *float64
    Load5          *float64
    Load15         *float64
    MemTotal       *uint64
    MemUsed        *uint64
    MemPercent     *float64
    SwapTotal      *uint64
    SwapUsed       *uint64
    Disks          []DiskUsage
    NetBytesSent   *uint64
    NetBytesRecv   *uint64

    // Custom metrics (stored as JSON in metric.extra_json)
    Extra map[string]interface{}
}
```

Only set fields your observer is responsible for. `nil` fields are ignored during merge.

### Registering an Observer

In `cmd/run.go` (or a dedicated registration file):

```go
registry.Register(&observers.TemperatureObserver{})
```

The registry calls `Detect()` at startup. If it returns `true`, the observer is started.

## Writing a Probe

Probes monitor specific services. They emit events and can respond to queries from other nodes.

```go
package probes

import (
    "context"
    "time"

    "github.com/dx111ge/homelabmon/internal/plugin"
)

type DockerProbe struct {
    client *docker.Client
}

var _ plugin.Probe = (*DockerProbe)(nil)

func (p *DockerProbe) Name() string           { return "docker" }
func (p *DockerProbe) Type() plugin.PluginType { return plugin.TypeProbe }
func (p *DockerProbe) Interval() time.Duration { return 30 * time.Second }

func (p *DockerProbe) Detect() bool {
    // Check if Docker daemon is accessible
    cli, err := docker.NewClientWithOpts(docker.FromEnv)
    if err != nil {
        return false
    }
    _, err = cli.Ping(context.Background())
    if err != nil {
        return false
    }
    p.client = cli
    return true
}

func (p *DockerProbe) Start(ctx context.Context) error { return nil }
func (p *DockerProbe) Stop() error {
    if p.client != nil {
        return p.client.Close()
    }
    return nil
}

func (p *DockerProbe) Collect(ctx context.Context) ([]plugin.Event, error) {
    containers, err := p.client.ContainerList(ctx, container.ListOptions{All: true})
    if err != nil {
        return nil, err
    }

    var events []plugin.Event
    for _, c := range containers {
        events = append(events, plugin.Event{
            Probe:     "docker",
            Type:      "container_status",
            Timestamp: time.Now().UTC(),
            Severity:  severityForState(c.State),
            Data: map[string]interface{}{
                "container_id":   c.ID[:12],
                "name":           c.Names[0],
                "image":          c.Image,
                "state":          c.State,
                "status":         c.Status,
            },
        })
    }
    return events, nil
}

func (p *DockerProbe) Query(method string, params json.RawMessage) (any, error) {
    // Handle queries from other nodes or the LLM
    switch method {
    case "containers":
        return p.client.ContainerList(context.Background(), container.ListOptions{All: true})
    case "stats":
        var req struct{ ContainerID string }
        json.Unmarshal(params, &req)
        return p.client.ContainerStats(context.Background(), req.ContainerID, false)
    default:
        return nil, fmt.Errorf("unknown method: %s", method)
    }
}
```

### Event Structure

```go
type Event struct {
    Probe     string                 `json:"probe"`     // plugin name
    Type      string                 `json:"type"`      // event type
    Timestamp time.Time              `json:"timestamp"`
    Severity  string                 `json:"severity"`  // info, warning, critical
    Data      map[string]interface{} `json:"data"`      // event-specific payload
}
```

## Writing a Scanner

Scanners discover passive devices on the network. They return discovered devices that are stored in the CMDB.

```go
package scanners

import (
    "context"
    "time"

    "github.com/dx111ge/homelabmon/internal/plugin"
)

type ARPScanner struct {
    subnet string
}

var _ plugin.Scanner = (*ARPScanner)(nil)

func (s *ARPScanner) Name() string           { return "arp" }
func (s *ARPScanner) Type() plugin.PluginType { return plugin.TypeScanner }
func (s *ARPScanner) Interval() time.Duration { return 5 * time.Minute }

func (s *ARPScanner) Detect() bool {
    // Only run if --scan is enabled (checked via config)
    return config.ScanEnabled()
}

func (s *ARPScanner) Start(ctx context.Context) error {
    s.subnet = detectLocalSubnet() // e.g., "192.168.1.0/24"
    return nil
}

func (s *ARPScanner) Stop() error { return nil }

func (s *ARPScanner) Scan(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
    // Perform ARP scan on local subnet
    results := arpScan(s.subnet)

    var devices []plugin.DiscoveredDevice
    for _, r := range results {
        devices = append(devices, plugin.DiscoveredDevice{
            MAC:        r.MAC,
            IP:         r.IP,
            Hostname:   resolveHostname(r.IP),
            Vendor:     lookupOUI(r.MAC),
            DeviceType: classifyDevice(r.MAC, r.Hostname),
            Source:     "arp",
        })
    }
    return devices, nil
}
```

### DiscoveredDevice Structure

```go
type DiscoveredDevice struct {
    MAC        string `json:"mac"`
    IP         string `json:"ip"`
    Hostname   string `json:"hostname"`
    Vendor     string `json:"vendor"`       // from OUI database
    DeviceType string `json:"device_type"`  // phone, tablet, tv, iot, printer, etc.
    Source     string `json:"source"`       // arp, mdns, snmp, etc.
}
```

## Writing an Integration

Integrations connect to external APIs (controllers, dashboards) and pull device/state data.

```go
package integrations

import (
    "context"
    "time"

    "github.com/dx111ge/homelabmon/internal/plugin"
)

type UnifiIntegration struct {
    url      string
    username string
    password string
    client   *unifi.Client
}

var _ plugin.Integration = (*UnifiIntegration)(nil)

func (u *UnifiIntegration) Name() string           { return "unifi" }
func (u *UnifiIntegration) Type() plugin.PluginType { return plugin.TypeIntegration }
func (u *UnifiIntegration) Interval() time.Duration { return 5 * time.Minute }

func (u *UnifiIntegration) Detect() bool {
    // Only activate if configured
    return u.url != ""
}

func (u *UnifiIntegration) Configure(config map[string]string) error {
    u.url = config["url"]
    u.username = config["username"]
    u.password = config["password"]
    return nil
}

func (u *UnifiIntegration) Start(ctx context.Context) error {
    client, err := unifi.NewClient(u.url, u.username, u.password)
    if err != nil {
        return err
    }
    u.client = client
    return nil
}

func (u *UnifiIntegration) Stop() error { return nil }

func (u *UnifiIntegration) Sync(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
    clients, err := u.client.GetClients()
    if err != nil {
        return nil, err
    }

    var devices []plugin.DiscoveredDevice
    for _, c := range clients {
        devices = append(devices, plugin.DiscoveredDevice{
            MAC:        c.MAC,
            IP:         c.IP,
            Hostname:   c.Hostname,
            Vendor:     c.OUI,
            DeviceType: mapUnifiDeviceType(c.DevCategory),
            Source:     "unifi",
        })
    }
    return devices, nil
}
```

### Integration Configuration

Integrations are configured via the **Settings UI** at `/ui/settings`. Credentials are encrypted with AES-256-GCM and stored in SQLite. No passwords in CLI flags or environment variables.

The Settings page provides:
- **Add Integration** form (type, name, URL, username, password)
- **Test** button to verify connectivity
- **Sync** button for manual device pull
- **Delete** button to remove integration and encrypted secrets

Integrations auto-sync every 5 minutes in the background.

### Implemented Integration: FRITZ!Box (TR-064)

```go
fb := integrations.NewFritzBox("http://192.168.178.1:49000", "admin", "password")
devices, err := fb.Sync(ctx)  // returns []DiscoveredDevice
```

Uses TR-064 SOAP API with HTTP Digest Authentication. Iterates `GetGenericHostEntry` for all known hosts. Devices are deduplicated with existing ARP/mDNS entries via MAC-based deterministic IDs.

## Plugin Lifecycle

```
  Register()          Detect()         Start()
  ────────► REGISTERED ────────► DETECTED ────────► RUNNING
                         │                              │
                         │ Detect()=false                │ error or Stop()
                         ▼                              ▼
                       SKIPPED                        STOPPED/FAILED
```

1. **Register**: Plugin is added to the registry (at compile time or startup)
2. **Detect**: Registry calls `Detect()` on each plugin. Returns `false` = skipped.
3. **Start**: Plugin sets up resources (connections, file handles, etc.)
4. **Running**: Registry calls `Collect()`/`Scan()`/`Sync()` at `Interval()`
5. **Stop**: Called on shutdown. Clean up resources.

If `Collect`/`Scan`/`Sync` returns an error, it's logged but the plugin stays running. Persistent errors can trigger a state change to FAILED.

## Plugin Registry API

```go
registry := plugin.NewRegistry()

// Register plugins
registry.Register(&observers.CPUObserver{})
registry.Register(&observers.MemoryObserver{})
registry.Register(&probes.DockerProbe{})
registry.Register(&scanners.ARPScanner{})
registry.Register(&integrations.UnifiIntegration{})

// Auto-detect and start
registry.DetectAndStart(ctx)

// Query at runtime
running := registry.ListByType(plugin.TypeObserver)
docker := registry.Get("docker")

// Shutdown
registry.StopAll()
```

## Porting Plugins to BYKT/Aria

The plugin interfaces are designed to be compatible. To reuse a HomelabMon plugin in BYKT:

1. The `Plugin` base interface maps to BYKT's `Probe` interface
2. `Observer.Collect()` maps to BYKT's resource probe pattern
3. `Probe.Collect()` returns `[]Event` -- same structure as BYKT's `[]probe.Event`
4. `Probe.Query()` maps to BYKT's A2A query handler
5. `Scanner.Scan()` is HomelabMon-specific (BYKT uses the Twin for network awareness)
6. `Integration.Sync()` is HomelabMon-specific (BYKT uses its own integration framework)

### Key Differences

| Aspect | HomelabMon | BYKT/Aria |
|--------|-------------|-----------|
| Event delivery | Return from Collect() | Event handler callback |
| Queries | Query() method | A2A JSON-RPC server |
| Config | YAML file | YAML file (same format) |
| Storage | Local SQLite | Local SQLite + remote Twin (Redis) |
| Lifecycle | Same: Detect -> Start -> Collect loop -> Stop | Same pattern |

To maximize reuse, keep plugin logic in a shared package and use thin adapters for each system.

## Naming Conventions

- Plugin names: lowercase, hyphenated: `cpu`, `docker`, `arp-scanner`, `unifi`
- Event types: snake_case: `container_status`, `resource_snapshot`, `device_discovered`
- Severity: `info`, `warning`, `critical`
- Device types: `server`, `desktop`, `laptop`, `phone`, `tablet`, `tv`, `iot`, `printer`, `camera`, `router`, `switch`, `ap`, `nas`, `other`

## Testing Plugins

```go
func TestTemperatureObserver_Detect(t *testing.T) {
    obs := &TemperatureObserver{}
    // Detect should return true/false based on platform
    if runtime.GOOS == "linux" {
        assert.True(t, obs.Detect())
    }
}

func TestTemperatureObserver_Collect(t *testing.T) {
    obs := &TemperatureObserver{}
    if !obs.Detect() {
        t.Skip("temperature sensors not available")
    }

    result, err := obs.Collect(context.Background())
    assert.NoError(t, err)
    assert.NotNil(t, result.Extra["cpu_temp"])
}
```

## External Plugins

External plugins are standalone executables that communicate with HomelabMon via JSON on stdin/stdout. This allows writing plugins in any language (Python, Node.js, Rust, etc.).

### Protocol

The plugin reads JSON request objects from stdin (one per line) and writes JSON response objects to stdout (one per line).

**Request format:**
```json
{"method": "name", "params": null}
{"method": "collect", "params": null}
```

**Response format:**
```json
{"result": "my-plugin", "error": ""}
{"result": {"extra": {"temperature": 42.5}}, "error": ""}
```

### Required Methods

Every external plugin must respond to these methods:

| Method | Returns | Description |
|--------|---------|-------------|
| `name` | `string` | Unique plugin identifier |
| `type` | `string` | `"observer"` or `"scanner"` |
| `detect` | `bool` | Can this plugin run on this system? |
| `interval` | `number` | Collection interval in seconds |

### Observer Plugins

Respond to `collect` method, returning an `ObserverResult`-compatible JSON:

```json
{"result": {"extra": {"cpu_temp": 65.2, "gpu_temp": 72.0}}, "error": ""}
```

### Scanner Plugins

Respond to `scan` method, returning `[]DiscoveredDevice`:

```json
{"result": [{"ip": "192.168.1.50", "mac": "aa:bb:cc:dd:ee:ff", "hostname": "mydevice", "vendor": "Acme", "device_type": "iot", "source": "my-scanner"}], "error": ""}
```

### Installation

Place the plugin executable in `~/.homelabmon/plugins/` (or `<data-dir>/plugins/`). It will be auto-loaded at startup.

### Example (Python)

```python
#!/usr/bin/env python3
import json, sys

for line in sys.stdin:
    req = json.loads(line)
    method = req["method"]

    if method == "name":
        resp = {"result": "temperature", "error": ""}
    elif method == "type":
        resp = {"result": "observer", "error": ""}
    elif method == "detect":
        resp = {"result": True, "error": ""}
    elif method == "interval":
        resp = {"result": 30, "error": ""}
    elif method == "collect":
        temp = read_cpu_temp()  # your implementation
        resp = {"result": {"extra": {"cpu_temp": temp}}, "error": ""}
    else:
        resp = {"result": None, "error": f"unknown method: {method}"}

    print(json.dumps(resp), flush=True)
```

## Checklist for New Plugins

- [ ] Implements the correct interface (Observer/Probe/Scanner/Integration)
- [ ] `Name()` returns a unique, lowercase identifier
- [ ] `Detect()` checks if the plugin can run (don't assume availability)
- [ ] `Interval()` returns a sensible collection frequency
- [ ] `Stop()` cleans up all resources (connections, goroutines, file handles)
- [ ] Errors are returned, not panicked
- [ ] Context cancellation is respected in long-running operations
- [ ] Compile-time interface check: `var _ plugin.Observer = (*MyPlugin)(nil)`
- [ ] Tests cover Detect, Collect, and edge cases
- [ ] Registered in `cmd/run.go` or auto-registration file
