//go:build windows

package observers

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/Microsoft/go-winio"
)

// Windows named pipe paths for Docker Engine.
// Docker Desktop (WSL2 backend) uses dockerDesktopLinuxEngine;
// traditional Docker for Windows uses docker_engine.
var windowsDockerPipes = []string{
	"//./pipe/dockerDesktopLinuxEngine",
	"//./pipe/docker_engine",
}

func dockerHTTPClient() *http.Client {
	// Find the first pipe that we can open.
	pipePath := ""
	for _, p := range windowsDockerPipes {
		conn, err := winio.DialPipe(p, nil)
		if err == nil {
			conn.Close()
			pipePath = p
			break
		}
	}
	if pipePath == "" {
		return nil
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			deadline, ok := ctx.Deadline()
			var timeout *time.Duration
			if ok {
				t := time.Until(deadline)
				timeout = &t
			}
			return winio.DialPipe(pipePath, timeout)
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
}
