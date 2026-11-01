package jobs

import (
	"encoding/json"
	"unicode/utf8"
)

type Payload struct {
	DisplayName, UUID, Job string
	Attempts, MaxTries     int
	Command                string
}

func DecodePayload(raw string) (Payload, error) {
	var v struct {
		DisplayName string `json:"displayName"`
		UUID        string `json:"uuid"`
		Job         string `json:"job"`
		Attempts    int    `json:"attempts"`
		MaxTries    int    `json:"maxTries"`
		Data        struct {
			Command string `json:"command"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return Payload{}, err
	}
	c := v.Data.Command
	if utf8.RuneCountInString(c) > 2000 {
		c = string([]rune(c)[:2000])
	}
	return Payload{v.DisplayName, v.UUID, v.Job, v.Attempts, v.MaxTries, c}, nil
}
