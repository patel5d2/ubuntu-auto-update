package models

type Webhook struct {
	ID    int32  `json:"id"`
	URL   string `json:"url"`
	Event string `json:"event"`
}