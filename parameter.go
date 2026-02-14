package gobank

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Parameter represents a time-varying value for an account.
type Parameter struct {
	AccountID   int64
	Key         string
	Value       string
	EffectiveAt time.Time
}

type paramKey struct {
	AccountID int64
	Key       string
}

// ParameterStore holds time-varying per-account parameters with effective dates.
type ParameterStore struct {
	mu     sync.RWMutex
	params map[paramKey][]Parameter
}

func NewParameterStore() *ParameterStore {
	return &ParameterStore{
		params: make(map[paramKey][]Parameter),
	}
}

// Set records a parameter value effective from the given time.
func (ps *ParameterStore) Set(accountID int64, key, value string, effectiveAt time.Time) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	pk := paramKey{AccountID: accountID, Key: key}
	ps.params[pk] = append(ps.params[pk], Parameter{
		AccountID:   accountID,
		Key:         key,
		Value:       value,
		EffectiveAt: effectiveAt,
	})
}

// Get retrieves the parameter value in effect at the given time.
// Returns the value and true if found, or empty string and false if not.
func (ps *ParameterStore) Get(accountID int64, key string, asOf time.Time) (string, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	pk := paramKey{AccountID: accountID, Key: key}
	entries := ps.params[pk]
	var best *Parameter
	for i := range entries {
		if !entries[i].EffectiveAt.After(asOf) {
			if best == nil || entries[i].EffectiveAt.After(best.EffectiveAt) {
				best = &entries[i]
			}
		}
	}
	if best == nil {
		return "", false
	}
	return best.Value, true
}

// GetFloat64 retrieves a parameter as float64.
func (ps *ParameterStore) GetFloat64(accountID int64, key string, asOf time.Time) (float64, error) {
	val, ok := ps.Get(accountID, key, asOf)
	if !ok {
		return 0, fmt.Errorf("parameter %q not found for account %d at %s", key, accountID, asOf)
	}
	return strconv.ParseFloat(val, 64)
}
