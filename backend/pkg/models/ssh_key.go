package models

type SSHKey struct {
	HostID     int32  `json:"host_id"`
	PrivateKey string `json:"private_key"`
}