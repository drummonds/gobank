package gobank

import (
	"time"

	luca "github.com/drummonds/go-luca"
)

// ManagedAccount wraps a go-luca Account with banking lifecycle state.
type ManagedAccount struct {
	Account      *luca.Account
	BehaviorName string
	CustomerID   string
	Status       AccountStatus
	OpenedAt     time.Time
	ClosedAt     time.Time
}
