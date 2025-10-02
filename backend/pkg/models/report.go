package models

// HostReport is the structure of the report sent by the agent.
type HostReport struct {
	Hostname      string `json:"hostname"`
	UpdateOutput  string `json:"update_output"`
	UpgradeOutput string `json:"upgrade_output"`
}
