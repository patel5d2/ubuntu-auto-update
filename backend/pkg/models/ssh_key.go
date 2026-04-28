package models

type SSHKey struct {
	HostID     int32  `json:"host_id" db:"host_id"`
	PrivateKey string `json:"private_key" db:"private_key"`
}
