package utils

type Mailbox struct {
	Name     string `json:"name"`
	Delete   bool   `json:"delete"`
	Export   bool   `json:"export"`
	Lifespan int    `json:"lifespan"`
}
