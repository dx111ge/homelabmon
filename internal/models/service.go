package models

import "time"

type DiscoveredService struct {
	ID           int64     `json:"id,omitempty"`
	HostID       string    `json:"host_id"`
	Name         string    `json:"name"`
	Port         int       `json:"port"`
	Protocol     string    `json:"protocol"`
	Process      string    `json:"process"`
	Category     string    `json:"category"`
	Source       string    `json:"source"` // fingerprint, docker, manual
	ContainerID  string    `json:"container_id,omitempty"`
	ContainerImg string    `json:"container_image,omitempty"`
	Status       string    `json:"status"` // active, gone
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// ServiceCategoryIcon returns a Font Awesome icon for a service category.
func ServiceCategoryIcon(category string) string {
	switch category {
	case "web", "proxy":
		return "fa-solid fa-globe"
	case "database":
		return "fa-solid fa-database"
	case "cache":
		return "fa-solid fa-bolt"
	case "media":
		return "fa-solid fa-film"
	case "monitoring":
		return "fa-solid fa-chart-line"
	case "container":
		return "fa-brands fa-docker"
	case "dns":
		return "fa-solid fa-shield-halved"
	case "automation":
		return "fa-solid fa-house-signal"
	case "mqtt":
		return "fa-solid fa-tower-broadcast"
	case "sync":
		return "fa-solid fa-rotate"
	case "download":
		return "fa-solid fa-download"
	case "remote-access":
		return "fa-solid fa-terminal"
	case "ai":
		return "fa-solid fa-brain"
	case "network":
		return "fa-solid fa-network-wired"
	default:
		return "fa-solid fa-cube"
	}
}
