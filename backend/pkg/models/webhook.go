package models

type Webhook struct {
	ID    int32  `json:"id" db:"id"`
	URL   string `json:"url" db:"url"`
	Event string `json:"event" db:"event"`
}
