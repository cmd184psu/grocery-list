package model

import "time"

type ItemState string

const (
	StateNeeded    ItemState = "needed"
	StateCheck     ItemState = "check"
	StateNotNeeded ItemState = "not_needed"

	// NoGroup is the virtual group name for orphaned items.
	// It is never stored in the groups list; items carry it in their Group field.
	NoGroup = "No Group"
)

type Item struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Group     string    `json:"group"`
	State     ItemState `json:"state"`
	Completed bool      `json:"completed"`
	Order     int       `json:"order"`
	CreatedAt time.Time `json:"created_at"`
}
