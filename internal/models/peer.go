package models

import "time"

type NodeIdentity struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	BindAddr string `json:"bind_addr"`
	Version  string `json:"version"`
	DataDir  string `json:"-"`
}

type PeerInfo struct {
	ID            string     `json:"id"`
	Hostname      string     `json:"hostname"`
	Address       string     `json:"address"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	Status        string     `json:"status"`
	Version       string     `json:"version"`
	EnrolledAt    time.Time  `json:"enrolled_at"`
}

type Heartbeat struct {
	NodeID    string           `json:"node_id"`
	Hostname  string           `json:"hostname"`
	Version   string           `json:"version"`
	Timestamp time.Time        `json:"timestamp"`
	Host      *Host            `json:"host,omitempty"`
	Metric    *MetricSnapshot  `json:"metric,omitempty"`
	Services  []DiscoveredService `json:"services,omitempty"`
	Peers     []string         `json:"peers,omitempty"`
}
