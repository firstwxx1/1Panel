package model

import "time"

// Node is a managed child machine (Kind "node") or a remote panel kept as a bookmark (Kind "panel").
type Node struct {
	BaseModel
	Name        string `gorm:"uniqueIndex;not null" json:"name"`
	Kind        string `gorm:"index;default:node" json:"kind"`
	Addr        string `json:"addr"`
	Port        int    `json:"port"`
	GroupID     uint   `json:"groupID"`
	Description string `json:"description"`
	IsFavorite  bool   `json:"isFavorite"`

	// panel bookmarks
	Protocol string `json:"protocol"`
	Entrance string `json:"entrance"`
	APIKey   string `json:"-"`

	// child nodes
	Status  string `json:"status"`
	Message string `json:"message"`
	// ProxyID is sent to the agent on every request; unbinding rotates it on the agent side
	ProxyID string `json:"-"`
	// JoinToken lets a one-click install fetch its certificate once
	JoinToken string `json:"-"`

	Version       string    `json:"version"`
	SystemVersion string    `json:"systemVersion"`
	Hostname      string    `json:"hostname"`
	LastSeen      time.Time `json:"lastSeen"`

	CPUTotal          int     `json:"cpuTotal"`
	CPUUsedPercent    float64 `json:"cpuUsedPercent"`
	MemoryTotal       uint64  `json:"memoryTotal"`
	MemoryUsedPercent float64 `json:"memoryUsedPercent"`
	Load1             float64 `json:"load1"`
}
