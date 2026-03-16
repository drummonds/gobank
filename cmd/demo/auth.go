package main

import (
	"sync"
	"time"
)

// Role represents a user role in the demo application.
type Role string

const (
	RoleAdmin           Role = "admin"
	RoleAuditor         Role = "auditor"
	RoleCustomerService Role = "cs"
	RoleReadOnly        Role = "readonly"
)

// RoleLabel returns a human-readable label for the role.
func (r Role) Label() string {
	switch r {
	case RoleAdmin:
		return "Admin"
	case RoleAuditor:
		return "Auditor"
	case RoleCustomerService:
		return "Customer Service"
	case RoleReadOnly:
		return "Read Only"
	default:
		return "Admin"
	}
}

// Can returns whether the role has permission for the given action.
func (r Role) Can(action string) bool {
	switch action {
	case "sim_controls":
		return r == RoleAdmin
	case "settings":
		return r == RoleAdmin
	case "export":
		return r == RoleAdmin
	case "send_payment":
		return r == RoleAdmin || r == RoleCustomerService
	case "view_pii":
		return r != RoleReadOnly
	case "buy_gilt":
		return r == RoleAdmin
	default:
		return false
	}
}

// ValidRole returns true if the string is a known role.
func ValidRole(s string) bool {
	switch Role(s) {
	case RoleAdmin, RoleAuditor, RoleCustomerService, RoleReadOnly:
		return true
	}
	return false
}

type sessionData struct {
	PIIExpiry time.Time
	Role      Role
}

// AuthStore manages simulated PII authorization sessions.
type AuthStore struct {
	mu       sync.Mutex
	sessions map[string]*sessionData
	ttl      time.Duration
}

func NewAuthStore(ttl time.Duration) *AuthStore {
	return &AuthStore{
		sessions: make(map[string]*sessionData),
		ttl:      ttl,
	}
}

func (as *AuthStore) getOrCreate(sessionID string) *sessionData {
	sd, ok := as.sessions[sessionID]
	if !ok {
		sd = &sessionData{Role: RoleAdmin}
		as.sessions[sessionID] = sd
	}
	return sd
}

func (as *AuthStore) Authorize(sessionID string) {
	as.mu.Lock()
	sd := as.getOrCreate(sessionID)
	sd.PIIExpiry = time.Now().Add(as.ttl)
	as.mu.Unlock()
}

func (as *AuthStore) Revoke(sessionID string) {
	as.mu.Lock()
	if sd, ok := as.sessions[sessionID]; ok {
		sd.PIIExpiry = time.Time{}
	}
	as.mu.Unlock()
}

func (as *AuthStore) IsAuthorized(sessionID string) bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	sd, ok := as.sessions[sessionID]
	if !ok {
		return false
	}
	if sd.PIIExpiry.IsZero() || time.Now().After(sd.PIIExpiry) {
		return false
	}
	return true
}

func (as *AuthStore) SetRole(sessionID string, role Role) {
	as.mu.Lock()
	sd := as.getOrCreate(sessionID)
	sd.Role = role
	as.mu.Unlock()
}

func (as *AuthStore) GetRole(sessionID string) Role {
	as.mu.Lock()
	defer as.mu.Unlock()
	sd, ok := as.sessions[sessionID]
	if !ok {
		return RoleAdmin
	}
	return sd.Role
}

// EffectivePII returns whether PII should be visible for this session.
// Auditor/CS: always true. ReadOnly: always false. Admin: TTL-based check.
func (as *AuthStore) EffectivePII(sessionID string) bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	sd, ok := as.sessions[sessionID]
	if !ok {
		return false
	}
	switch sd.Role {
	case RoleAuditor, RoleCustomerService:
		return true
	case RoleReadOnly:
		return false
	default: // Admin
		if sd.PIIExpiry.IsZero() || time.Now().After(sd.PIIExpiry) {
			return false
		}
		return true
	}
}
