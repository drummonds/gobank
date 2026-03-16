package gobank

import (
	"time"

	luca "github.com/drummonds/go-luca"
)

// AccountStatus represents the lifecycle state of a managed account.
type AccountStatus int

const (
	StatusPending AccountStatus = iota
	StatusActive
	StatusPendingClosure
	StatusClosed
)

func (s AccountStatus) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusActive:
		return "Active"
	case StatusPendingClosure:
		return "PendingClosure"
	case StatusClosed:
		return "Closed"
	default:
		return "Unknown"
	}
}

// EventContext provides context to account behavior hooks.
type EventContext struct {
	Sim      *Simulation
	Account  *ManagedAccount
	AsOfDate time.Time
}

// AccountBehavior defines the required lifecycle hooks for an account type.
type AccountBehavior interface {
	Name() string
	OnActivate(ctx EventContext) error
	OnOpen(ctx EventContext) error
	OnClose(ctx EventContext) error
	EndOfDay(ctx EventContext) error
}

// MovementHook is an optional interface for behaviors that need pre/post movement processing.
type MovementHook interface {
	PreMovement(ctx EventContext, fromID, toID string, amount luca.Amount) error
	PostMovement(ctx EventContext, fromID, toID string, amount luca.Amount) error
}

// ParameterHook is an optional interface for behaviors that react to parameter changes.
type ParameterHook interface {
	PreParameterChange(ctx EventContext, key, value string) error
	PostParameterChange(ctx EventContext, key, value string) error
}

// PendingClosureHook is an optional interface for behaviors with pending closure logic.
type PendingClosureHook interface {
	OnPendingClosure(ctx EventContext) error
}
