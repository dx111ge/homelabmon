package models

import "time"

type NodeIdentity struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	BindAddr string `json:"bind_addr"`
	Version  string `json:"version"`
	DataDir  string `json:"-"`
	Site     string `json:"site,omitempty"`
}

type PeerInfo struct {
	ID            string     `json:"id"`
	Hostname      string     `json:"hostname"`
	Address       string     `json:"address"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	Status        string     `json:"status"`
	Version       string     `json:"version"`
	EnrolledAt    time.Time  `json:"enrolled_at"`
	Site          string     `json:"site,omitempty"`
}

// PeerAddr is a lightweight peer reference exchanged in heartbeats for gossip discovery.
type PeerAddr struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Site    string `json:"site,omitempty"`
}

type Heartbeat struct {
	NodeID    string              `json:"node_id"`
	Hostname  string              `json:"hostname"`
	Address   string              `json:"address,omitempty"` // sender's listening address (host:port)
	Version   string              `json:"version"`
	Site      string              `json:"site,omitempty"`
	Timestamp time.Time           `json:"timestamp"`
	Host      *Host               `json:"host,omitempty"`
	Metric    *MetricSnapshot     `json:"metric,omitempty"`
	Services  []DiscoveredService `json:"services,omitempty"`
	KnownPeers []PeerAddr         `json:"known_peers,omitempty"`
}
