package main

import (
	"fmt"
	"sort"
	"sync"
)

// PIIInput is the input struct for storing PII fields.
type PIIInput struct {
	Name    string
	NI      string
	DOB     string
	Address string
	Email   string
	Phone   string
}

// PIIData is the output struct for retrieving decrypted PII fields.
type PIIData struct {
	Name    string
	NI      string
	DOB     string
	Address string
	Email   string
	Phone   string
}

// CustomerPII holds encrypted PII for a single customer.
type CustomerPII struct {
	CustomerID       string
	EncryptedName    string
	EncryptedNI      string
	EncryptedDOB     string
	EncryptedAddress string
	EncryptedEmail   string
	EncryptedPhone   string
}

// PIIStore is a thread-safe encrypted PII store.
type PIIStore struct {
	mu    sync.RWMutex
	store map[string]CustomerPII
}

func NewPIIStore() *PIIStore {
	return &PIIStore{store: make(map[string]CustomerPII)}
}

func (ps *PIIStore) Store(id string, pii PIIInput) error {
	encName, err := encryptPII(pii.Name)
	if err != nil {
		return fmt.Errorf("encrypt name: %w", err)
	}
	encNI, err := encryptPII(pii.NI)
	if err != nil {
		return fmt.Errorf("encrypt ni: %w", err)
	}
	encDOB, err := encryptPII(pii.DOB)
	if err != nil {
		return fmt.Errorf("encrypt dob: %w", err)
	}
	encAddr, err := encryptPII(pii.Address)
	if err != nil {
		return fmt.Errorf("encrypt address: %w", err)
	}
	encEmail, err := encryptPII(pii.Email)
	if err != nil {
		return fmt.Errorf("encrypt email: %w", err)
	}
	encPhone, err := encryptPII(pii.Phone)
	if err != nil {
		return fmt.Errorf("encrypt phone: %w", err)
	}
	ps.mu.Lock()
	ps.store[id] = CustomerPII{
		CustomerID:       id,
		EncryptedName:    encName,
		EncryptedNI:      encNI,
		EncryptedDOB:     encDOB,
		EncryptedAddress: encAddr,
		EncryptedEmail:   encEmail,
		EncryptedPhone:   encPhone,
	}
	ps.mu.Unlock()
	return nil
}

func (ps *PIIStore) Retrieve(id string) (PIIData, error) {
	ps.mu.RLock()
	pii, ok := ps.store[id]
	ps.mu.RUnlock()
	if !ok {
		return PIIData{}, fmt.Errorf("customer %s not found in PII store", id)
	}
	name, err := decryptPII(pii.EncryptedName)
	if err != nil {
		return PIIData{}, fmt.Errorf("decrypt name: %w", err)
	}
	ni, err := decryptPII(pii.EncryptedNI)
	if err != nil {
		return PIIData{}, fmt.Errorf("decrypt ni: %w", err)
	}
	dob, err := decryptPII(pii.EncryptedDOB)
	if err != nil {
		return PIIData{}, fmt.Errorf("decrypt dob: %w", err)
	}
	addr, err := decryptPII(pii.EncryptedAddress)
	if err != nil {
		return PIIData{}, fmt.Errorf("decrypt address: %w", err)
	}
	email, err := decryptPII(pii.EncryptedEmail)
	if err != nil {
		return PIIData{}, fmt.Errorf("decrypt email: %w", err)
	}
	phone, err := decryptPII(pii.EncryptedPhone)
	if err != nil {
		return PIIData{}, fmt.Errorf("decrypt phone: %w", err)
	}
	return PIIData{
		Name:    name,
		NI:      ni,
		DOB:     dob,
		Address: addr,
		Email:   email,
		Phone:   phone,
	}, nil
}

// RetrieveName returns only the decrypted name for a customer.
func (ps *PIIStore) RetrieveName(id string) string {
	pii, err := ps.Retrieve(id)
	if err != nil {
		return id // fallback to ID
	}
	return pii.Name
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
