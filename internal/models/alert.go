package models

import "time"

type Alert struct {
	ID         int64      `json:"id"`
	HostID     string     `json:"host_id"`
	Severity   string     `json:"severity"` // info, warning, critical
	Category   string     `json:"category"` // cpu, memory, disk, service, peer
	Message    string     `json:"message"`
	Details    string     `json:"details"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
