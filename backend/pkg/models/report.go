package models

import "time"

// HostReport is the payload the Rust agent POSTs to /api/v1/report. The shape
// must stay in sync with the agent's `HostReport` struct in
// agent/src/main.rs — the JSON tags here match its serde field names.
//
// Previously this was a flat {hostname, update_output, upgrade_output} struct,
// which failed to decode the agent's nested payload and dropped everything but
// the hostname. Now it mirrors the real wire format.
type HostReport struct {
	Hostname      string        `json:"hostname"`
	AgentVersion  string        `json:"agent_version"`
	Timestamp     time.Time     `json:"timestamp"`
	UpdateResults UpdateResults `json:"update_results"`
	SystemInfo    SystemInfo    `json:"system_info"`
	// Metrics is free-form agent telemetry; we don't persist it yet, but
	// accepting it keeps decoding from failing on the extra field.
	Metrics interface{} `json:"metrics"`
}

// UpdateResults mirrors agent/src/main.rs UpdateResults.
type UpdateResults struct {
	Success           bool    `json:"success"`
	DurationSeconds   float64 `json:"duration_seconds"`
	PackagesUpdated   int     `json:"packages_updated"`
	PackagesAvailable int     `json:"packages_available"`
	BytesDownloaded   int64   `json:"bytes_downloaded"`
	RebootRequired    bool    `json:"reboot_required"`
	ErrorMessage      *string `json:"error_message"`
	AptOutput         string  `json:"apt_output"`
	SnapOutput        *string `json:"snap_output"`
	FlatpakOutput     *string `json:"flatpak_output"`
}

// SystemInfo mirrors the subset of agent/src/main.rs SystemInfo we persist.
type SystemInfo struct {
	OsVersion     string `json:"os_version"`
	KernelVersion string `json:"kernel_version"`
	Architecture  string `json:"architecture"`
}
