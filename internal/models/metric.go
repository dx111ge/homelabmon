package models

import (
	"encoding/json"
	"time"
)

type MetricSnapshot struct {
	ID           int64     `json:"id,omitempty"`
	HostID       string    `json:"host_id"`
	CollectedAt  time.Time `json:"collected_at"`
	CPUPercent   float64   `json:"cpu_percent"`
	Load1        float64   `json:"load_1"`
	Load5        float64   `json:"load_5"`
	Load15       float64   `json:"load_15"`
	MemTotal     uint64    `json:"mem_total"`
	MemUsed      uint64    `json:"mem_used"`
	MemPercent   float64   `json:"mem_percent"`
	SwapTotal    uint64    `json:"swap_total"`
	SwapUsed     uint64    `json:"swap_used"`
	DiskJSON     string    `json:"disk_json"`
	NetBytesSent uint64    `json:"net_bytes_sent"`
	NetBytesRecv uint64    `json:"net_bytes_recv"`
}

type DiskUsage struct {
	Path        string  `json:"path"`
	Fstype      string  `json:"fstype"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

func (m *MetricSnapshot) Disks() []DiskUsage {
	var disks []DiskUsage
	json.Unmarshal([]byte(m.DiskJSON), &disks)
	return disks
}

func EncodeDiskUsages(disks []DiskUsage) string {
	b, _ := json.Marshal(disks)
	return string(b)
}
