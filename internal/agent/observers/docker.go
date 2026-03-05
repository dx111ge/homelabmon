package observers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
)

// DockerContainer holds key info about a running container.
type DockerContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Ports  []dockerPort      `json:"Ports"`
	Labels map[string]string `json:"Labels"`
}

type dockerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

// ContainerName returns the clean container name (without leading /).
func (c DockerContainer) ContainerName() string {
	if len(c.Names) == 0 {
		return c.ID[:12]
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// DockerObserver discovers running Docker containers via the Docker Engine API.
type DockerObserver struct {
	client *http.Client
	latest []DockerContainer
}

var _ plugin.Observer = (*DockerObserver)(nil)

func (o *DockerObserver) Name() string            { return "docker" }
func (o *DockerObserver) Type() models.PluginType { return models.PluginTypeObserver }
func (o *DockerObserver) Interval() time.Duration { return 60 * time.Second }
func (o *DockerObserver) Stop() error             { return nil }

// Latest returns the most recently collected containers.
func (o *DockerObserver) Latest() []DockerContainer { return o.latest }

// Detect checks if the Docker socket/pipe is accessible.
func (o *DockerObserver) Detect() bool {
	client := dockerHTTPClient()
	if client == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://docker/v1.47/_ping", nil)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (o *DockerObserver) Start(_ context.Context) error {
	o.client = dockerHTTPClient()
	if o.client == nil {
		return fmt.Errorf("docker socket not available")
	}
	return nil
}

func (o *DockerObserver) Collect(ctx context.Context) (*plugin.ObserverResult, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://docker/v1.47/containers/json?all=false", nil)
	resp, err := o.client.Do(req)
	if err != nil {
		o.latest = nil
		return nil, fmt.Errorf("docker api: %w", err)
	}
	defer resp.Body.Close()

	var containers []DockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("decode containers: %w", err)
	}

	o.latest = containers

	return &plugin.ObserverResult{
		Extra: map[string]interface{}{
			"docker_containers": len(containers),
		},
	}, nil
}

// ContainerAction performs start/stop/restart on a container by ID.
func (o *DockerObserver) ContainerAction(ctx context.Context, containerID, action string) error {
	if o.client == nil {
		return fmt.Errorf("docker client not initialized")
	}
	url := fmt.Sprintf("http://docker/v1.47/containers/%s/%s", containerID, action)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("docker %s: %w", action, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("docker %s returned %d", action, resp.StatusCode)
	}
	return nil
}

// dockerHTTPClient is defined in docker_unix.go / docker_windows.go
