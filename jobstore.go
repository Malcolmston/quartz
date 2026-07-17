package quartz

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Common store errors.
var (
	// ErrJobNotFound is returned when a job key is not present in the store.
	ErrJobNotFound = errors.New("quartz: job not found")
	// ErrTriggerNotFound is returned when a trigger key is not present.
	ErrTriggerNotFound = errors.New("quartz: trigger not found")
	// ErrJobExists is returned when storing a job whose key already exists.
	ErrJobExists = errors.New("quartz: job already exists")
	// ErrTriggerExists is returned when storing a trigger whose key already
	// exists.
	ErrTriggerExists = errors.New("quartz: trigger already exists")
)

// TriggerState enumerates the scheduling state of a stored trigger.
type TriggerState int

const (
	// TriggerStateNormal means the trigger is eligible to fire.
	TriggerStateNormal TriggerState = iota
	// TriggerStatePaused means the trigger will not fire until resumed.
	TriggerStatePaused
	// TriggerStateComplete means the trigger will not fire again.
	TriggerStateComplete
)

func (s TriggerState) String() string {
	switch s {
	case TriggerStatePaused:
		return "PAUSED"
	case TriggerStateComplete:
		return "COMPLETE"
	default:
		return "NORMAL"
	}
}

// StoredTrigger couples a Trigger with its scheduling state inside a JobStore.
type StoredTrigger struct {
	Trigger Trigger
	State   TriggerState
}

// JobStore persists JobDetails and Triggers. Implementations must be safe for
// concurrent use. The in-memory implementation is MemoryJobStore; a database
// backed store can be added by implementing this interface.
type JobStore interface {
	// StoreJob saves a JobDetail. If replace is false and the job already
	// exists, ErrJobExists is returned.
	StoreJob(detail *JobDetail, replace bool) error
	// RemoveJob deletes a job. It reports whether a job was removed.
	RemoveJob(key Key) (bool, error)
	// GetJob returns the JobDetail for a key.
	GetJob(key Key) (*JobDetail, error)
	// JobKeys returns all stored job keys.
	JobKeys() []Key

	// StoreTrigger saves a trigger in the given state. If replace is false
	// and the trigger exists, ErrTriggerExists is returned.
	StoreTrigger(trigger Trigger, state TriggerState, replace bool) error
	// RemoveTrigger deletes a trigger. It reports whether one was removed.
	RemoveTrigger(key Key) (bool, error)
	// GetTrigger returns the stored trigger for a key.
	GetTrigger(key Key) (*StoredTrigger, error)
	// TriggerKeys returns all stored trigger keys.
	TriggerKeys() []Key
	// TriggersForJob returns all triggers referencing the given job.
	TriggersForJob(jobKey Key) []*StoredTrigger

	// SetTriggerState updates the state of a trigger.
	SetTriggerState(key Key, state TriggerState) error

	// AcquireNextTriggers returns up to maxCount triggers in NORMAL state
	// whose next fire time is at or before noLaterThan, ordered by fire
	// time. It does not mutate state.
	AcquireNextTriggers(noLaterThan time.Time, maxCount int) []*StoredTrigger
}

// MemoryJobStore is an in-memory, mutex protected JobStore suitable for a
// single process scheduler.
type MemoryJobStore struct {
	mu       sync.RWMutex
	jobs     map[Key]*JobDetail
	triggers map[Key]*StoredTrigger
}

// NewMemoryJobStore constructs an empty in-memory store.
func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{
		jobs:     make(map[Key]*JobDetail),
		triggers: make(map[Key]*StoredTrigger),
	}
}

// StoreJob implements JobStore.
func (s *MemoryJobStore) StoreJob(detail *JobDetail, replace bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[detail.key]; ok && !replace {
		return ErrJobExists
	}
	s.jobs[detail.key] = detail
	return nil
}

// RemoveJob implements JobStore.
func (s *MemoryJobStore) RemoveJob(key Key) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.jobs[key]
	delete(s.jobs, key)
	return ok, nil
}

// GetJob implements JobStore.
func (s *MemoryJobStore) GetJob(key Key) (*JobDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.jobs[key]
	if !ok {
		return nil, ErrJobNotFound
	}
	return d, nil
}

// JobKeys implements JobStore.
func (s *MemoryJobStore) JobKeys() []Key {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]Key, 0, len(s.jobs))
	for k := range s.jobs {
		keys = append(keys, k)
	}
	sortKeys(keys)
	return keys
}

// StoreTrigger implements JobStore.
func (s *MemoryJobStore) StoreTrigger(trigger Trigger, state TriggerState, replace bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.triggers[trigger.Key()]; ok && !replace {
		return ErrTriggerExists
	}
	s.triggers[trigger.Key()] = &StoredTrigger{Trigger: trigger, State: state}
	return nil
}

// RemoveTrigger implements JobStore.
func (s *MemoryJobStore) RemoveTrigger(key Key) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.triggers[key]
	delete(s.triggers, key)
	return ok, nil
}

// GetTrigger implements JobStore.
func (s *MemoryJobStore) GetTrigger(key Key) (*StoredTrigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.triggers[key]
	if !ok {
		return nil, ErrTriggerNotFound
	}
	return t, nil
}

// TriggerKeys implements JobStore.
func (s *MemoryJobStore) TriggerKeys() []Key {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]Key, 0, len(s.triggers))
	for k := range s.triggers {
		keys = append(keys, k)
	}
	sortKeys(keys)
	return keys
}

// TriggersForJob implements JobStore.
func (s *MemoryJobStore) TriggersForJob(jobKey Key) []*StoredTrigger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*StoredTrigger
	for _, t := range s.triggers {
		if t.Trigger.JobKey() == jobKey {
			out = append(out, t)
		}
	}
	return out
}

// SetTriggerState implements JobStore.
func (s *MemoryJobStore) SetTriggerState(key Key, state TriggerState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.triggers[key]
	if !ok {
		return ErrTriggerNotFound
	}
	t.State = state
	return nil
}

// AcquireNextTriggers implements JobStore.
func (s *MemoryJobStore) AcquireNextTriggers(noLaterThan time.Time, maxCount int) []*StoredTrigger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ready []*StoredTrigger
	for _, t := range s.triggers {
		if t.State != TriggerStateNormal {
			continue
		}
		next := t.Trigger.NextFireTime()
		if next.IsZero() {
			continue
		}
		if !next.After(noLaterThan) {
			ready = append(ready, t)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].Trigger.NextFireTime().Before(ready[j].Trigger.NextFireTime())
	})
	if maxCount > 0 && len(ready) > maxCount {
		ready = ready[:maxCount]
	}
	return ready
}

func sortKeys(keys []Key) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Group != keys[j].Group {
			return keys[i].Group < keys[j].Group
		}
		return keys[i].Name < keys[j].Name
	})
}
