package models

import "time"

type Project struct {
	ID       int64      `json:"id"`
	Name     string     `json:"name"`
	ParentID *int64     `json:"parent_id"`
	Children []*Project `json:"children,omitempty"`
	Sessions []*Session `json:"sessions,omitempty"`

	// Computed
	TotalSeconds int64 `json:"total_seconds"`
}

type Session struct {
	ID        int64      `json:"id"`
	ProjectID int64      `json:"project_id"`
	Start     time.Time  `json:"start"`
	End       *time.Time `json:"end"`
	Note      string     `json:"note"`
	Duration  int64      `json:"duration"` // seconds, 0 if active
}

type Status struct {
	Active        bool     `json:"active"`
	Project       *Project `json:"project,omitempty"`
	Session       *Session `json:"session,omitempty"`
	Path          string   `json:"path"` // e.g. "Chess > Library"
	Elapsed       int64    `json:"elapsed_seconds"`
	Total         int64    `json:"total_seconds"`
	WaybarText    string   `json:"text"`
	WaybarTooltip string   `json:"tooltip"`
	WaybarClass   string   `json:"class"`
}

type ImportSession struct {
	Project  string `json:"project"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Duration int64  `json:"duration"`
}
