package models

import "time"

type PluginType string

const (
	PluginTypeObserver    PluginType = "observer"
	PluginTypeProbe       PluginType = "probe"
	PluginTypeScanner     PluginType = "scanner"
	PluginTypeIntegration PluginType = "integration"
)

type PluginState string

const (
	PluginRegistered PluginState = "registered"
	PluginRunning    PluginState = "running"
	PluginStopped    PluginState = "stopped"
	PluginFailed     PluginState = "failed"
	PluginSkipped    PluginState = "skipped"
)

type PluginInfo struct {
	Name     string      `json:"name"`
	Type     PluginType  `json:"type"`
	Status   PluginState `json:"status"`
	LastRun  *time.Time  `json:"last_run,omitempty"`
	ErrorMsg string      `json:"error_msg,omitempty"`
}
