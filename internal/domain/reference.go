package domain

import (
	"sync"
	"time"
)

// ReferenceData holds the current reference tick and epoch
type ReferenceData struct {
	mu        sync.RWMutex
	Tick      uint32
	Epoch     uint16
	Source    string
	UpdatedAt time.Time
}

// NewReferenceData creates a new ReferenceData instance
func NewReferenceData() *ReferenceData {
	return &ReferenceData{}
}

// Update updates the reference data atomically
func (r *ReferenceData) Update(tick uint32, epoch uint16, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Tick = tick
	r.Epoch = epoch
	r.Source = source
	r.UpdatedAt = time.Now()
}

// Get returns a copy of the current reference data
func (r *ReferenceData) Get() (tick uint32, epoch uint16, source string, updatedAt time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Tick, r.Epoch, r.Source, r.UpdatedAt
}

// GetTick returns the current reference tick
func (r *ReferenceData) GetTick() uint32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Tick
}

// GetEpoch returns the current reference epoch
func (r *ReferenceData) GetEpoch() uint16 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Epoch
}

// IsStale returns true if the reference data is older than the given duration
func (r *ReferenceData) IsStale(maxAge time.Duration) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return time.Since(r.UpdatedAt) > maxAge
}
