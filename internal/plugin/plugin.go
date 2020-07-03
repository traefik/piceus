package plugin

import "time"

// Plugin The plugin information.
type Plugin struct {
	ID            string                 `json:"id,omitempty"`
	Name          string                 `json:"name,omitempty"`
	DisplayName   string                 `json:"displayName,omitempty"`
	Author        string                 `json:"author,omitempty"`
	Type          string                 `json:"type,omitempty"`
	Import        string                 `json:"import,omitempty"`
	Compatibility string                 `json:"compatibility,omitempty"`
	Summary       string                 `json:"summary,omitempty"`
	Readme        string                 `json:"readme,omitempty"`
	LatestVersion string                 `json:"latestVersion,omitempty"`
	Versions      []string               `json:"versions,omitempty"`
	Stars         int                    `json:"stars,omitempty"`
	Snippet       map[string]interface{} `json:"snippet,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
}
