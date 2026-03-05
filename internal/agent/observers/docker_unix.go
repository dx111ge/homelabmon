//go:build !windows

package observers

import (
	"context"
	"net"
	"net/http"
	"time"
)

func dockerHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
}
