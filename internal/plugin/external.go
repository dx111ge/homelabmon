package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/rs/zerolog/log"
)

// ExternalPlugin wraps a subprocess that communicates via JSON on stdin/stdout.
// The subprocess receives JSON request objects and returns JSON response objects.
//
// Protocol:
//   Request:  {"method": "name"|"type"|"detect"|"interval"|"collect"|"scan"|"sync", "params": {...}}
//   Response: {"result": ..., "error": "..."}
//
// Supported plugin types: observer, scanner.
// The subprocess must respond to "name", "type", "detect", and "interval" methods.
// For observers: responds to "collect" returning ObserverResult-compatible JSON.
// For scanners: responds to "scan" returning []DiscoveredDevice-compatible JSON.
type ExternalPlugin struct {
	path    string
	args    []string
	cmd     *exec.Cmd
	stdin   *json.Encoder
	scanner *bufio.Scanner
	mu      sync.Mutex

	// Cached metadata (fetched once at init)
	name     string
	ptype    models.PluginType
	detected bool
	interval time.Duration
}

// externalRequest is sent to the subprocess.
type externalRequest struct {
	Method string      `json:"method"`
	Params interface{} `json:"params,omitempty"`
}

// externalResponse is received from the subprocess.
type externalResponse struct {
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

// NewExternalPlugin creates a wrapper for an external plugin binary.
func NewExternalPlugin(path string, args ...string) *ExternalPlugin {
	return &ExternalPlugin{
		path: path,
		args: args,
	}
}

// Init starts the subprocess and queries metadata. Must be called before other methods.
func (e *ExternalPlugin) Init() error {
	e.cmd = exec.Command(e.path, e.args...)
	stdin, err := e.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := e.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", e.path, err)
	}

	e.stdin = json.NewEncoder(stdin)
	e.scanner = bufio.NewScanner(stdout)
	e.scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB buffer

	// Query metadata
	nameResp, err := e.call("name", nil)
	if err != nil {
		e.kill()
		return fmt.Errorf("query name: %w", err)
	}
	json.Unmarshal(nameResp, &e.name)

	var typeStr string
	typeResp, err := e.call("type", nil)
	if err != nil {
		e.kill()
		return fmt.Errorf("query type: %w", err)
	}
	json.Unmarshal(typeResp, &typeStr)
	e.ptype = models.PluginType(typeStr)

	detectResp, err := e.call("detect", nil)
	if err != nil {
		e.kill()
		return fmt.Errorf("query detect: %w", err)
	}
	json.Unmarshal(detectResp, &e.detected)

	var intervalSecs float64
	intervalResp, err := e.call("interval", nil)
	if err != nil {
		e.kill()
		return fmt.Errorf("query interval: %w", err)
	}
	json.Unmarshal(intervalResp, &intervalSecs)
	e.interval = time.Duration(intervalSecs * float64(time.Second))
	if e.interval < time.Second {
		e.interval = 30 * time.Second
	}

	log.Info().Str("plugin", e.name).Str("type", typeStr).Bool("detected", e.detected).Msg("external plugin initialized")
	return nil
}

func (e *ExternalPlugin) call(method string, params interface{}) (json.RawMessage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.stdin.Encode(externalRequest{Method: method, Params: params}); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	if !e.scanner.Scan() {
		if err := e.scanner.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("plugin process closed stdout")
	}

	var resp externalResponse
	if err := json.Unmarshal(e.scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("plugin error: %s", resp.Error)
	}
	return resp.Result, nil
}

func (e *ExternalPlugin) kill() {
	if e.cmd != nil && e.cmd.Process != nil {
		e.cmd.Process.Kill()
	}
}

// Plugin interface implementation
func (e *ExternalPlugin) Name() string            { return e.name }
func (e *ExternalPlugin) Type() models.PluginType { return e.ptype }
func (e *ExternalPlugin) Detect() bool            { return e.detected }
func (e *ExternalPlugin) Interval() time.Duration { return e.interval }

func (e *ExternalPlugin) Start(_ context.Context) error { return nil }

func (e *ExternalPlugin) Stop() error {
	e.kill()
	if e.cmd != nil {
		return e.cmd.Wait()
	}
	return nil
}

// ExternalObserver wraps ExternalPlugin to implement the Observer interface.
type ExternalObserver struct {
	*ExternalPlugin
}

func (o *ExternalObserver) Collect(ctx context.Context) (*ObserverResult, error) {
	resp, err := o.call("collect", nil)
	if err != nil {
		return nil, err
	}
	var result ObserverResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("decode collect result: %w", err)
	}
	return &result, nil
}

// ExternalScanner wraps ExternalPlugin to implement the Scanner interface.
type ExternalScanner struct {
	*ExternalPlugin
}

func (s *ExternalScanner) Scan(ctx context.Context) ([]DiscoveredDevice, error) {
	resp, err := s.call("scan", nil)
	if err != nil {
		return nil, err
	}
	var devices []DiscoveredDevice
	if err := json.Unmarshal(resp, &devices); err != nil {
		return nil, fmt.Errorf("decode scan result: %w", err)
	}
	return devices, nil
}

// LoadExternalPlugin initializes an external plugin and returns the appropriate
// typed wrapper (ExternalObserver or ExternalScanner) based on its declared type.
// Returns nil if the plugin fails to initialize.
func LoadExternalPlugin(path string, args ...string) Plugin {
	ep := NewExternalPlugin(path, args...)
	if err := ep.Init(); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("failed to load external plugin")
		return nil
	}

	switch ep.ptype {
	case models.PluginTypeObserver:
		return &ExternalObserver{ep}
	case models.PluginTypeScanner:
		return &ExternalScanner{ep}
	default:
		log.Warn().Str("type", string(ep.ptype)).Str("path", path).Msg("unsupported external plugin type")
		ep.kill()
		return nil
	}
}
