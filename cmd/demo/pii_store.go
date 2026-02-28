package main

import (
	"fmt"
	"sort"
	"sync"
)

// CustomerPII holds encrypted PII for a single customer.
type CustomerPII struct {
	CustomerID    string
	EncryptedName string
	EncryptedNI   string
}

// PIIStore is a thread-safe encrypted PII store.
type PIIStore struct {
	mu    sync.RWMutex
	store map[string]CustomerPII
}

func NewPIIStore() *PIIStore {
	return &PIIStore{store: make(map[string]CustomerPII)}
}

func (ps *PIIStore) Store(id, name, ni string) error {
	encName, err := encryptPII(name)
	if err != nil {
		return fmt.Errorf("encrypt name: %w", err)
	}
	encNI, err := encryptPII(ni)
	if err != nil {
		return fmt.Errorf("encrypt ni: %w", err)
	}
	ps.mu.Lock()
	ps.store[id] = CustomerPII{
		CustomerID:    id,
		EncryptedName: encName,
		EncryptedNI:   encNI,
	}
	ps.mu.Unlock()
	return nil
}

func (ps *PIIStore) Retrieve(id string) (name, ni string, err error) {
	ps.mu.RLock()
	pii, ok := ps.store[id]
	ps.mu.RUnlock()
	if !ok {
		return "", "", fmt.Errorf("customer %s not found in PII store", id)
	}
	name, err = decryptPII(pii.EncryptedName)
	if err != nil {
		return "", "", fmt.Errorf("decrypt name: %w", err)
	}
	ni, err = decryptPII(pii.EncryptedNI)
	if err != nil {
		return "", "", fmt.Errorf("decrypt ni: %w", err)
	}
	return name, ni, nil
}

// RetrieveName returns only the decrypted name for a customer.
func (ps *PIIStore) RetrieveName(id string) string {
	name, _, err := ps.Retrieve(id)
	if err != nil {
		return id // fallback to ID
	}
	return name
}

func (ps *PIIStore) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.store)
}

func (ps *PIIStore) AllIDs() []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	ids := make([]string, 0, len(ps.store))
	for id := range ps.store {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (ps *PIIStore) Reset() {
	ps.mu.Lock()
	ps.store = make(map[string]CustomerPII)
	ps.mu.Unlock()
}
