package quartz

import "time"

// MisfirePolicy determines what a trigger does when the scheduler was unable to
// fire it on time (for example while paused or under load).
type MisfirePolicy int

const (
	// MisfireSmart lets the trigger choose a sensible default: SimpleTrigger
	// reschedules relative to now while preserving its remaining count, and
	// CronTrigger advances to the next valid time after now.
	MisfireSmart MisfirePolicy = iota
	// MisfireFireNow instructs the trigger to fire once immediately and then
	// resume its normal schedule.
	MisfireFireNow
	// MisfireIgnore instructs the trigger to fire for every missed time as
	// fast as possible until it has caught up with now.
	MisfireIgnore
	// MisfireDoNothing instructs the trigger to skip all missed fires and
	// compute its next fire time strictly after now.
	MisfireDoNothing
)

// Trigger describes when a job should fire. Implementations must be safe for
// use by a single scheduler goroutine at a time; the scheduler serializes all
// mutating calls (ComputeFirstFireTime and Triggered) per trigger.
type Trigger interface {
	// Key returns the trigger's identity.
	Key() Key
	// JobKey returns the key of the job this trigger fires.
	JobKey() Key

	// ComputeFirstFireTime initializes internal state and returns the first
	// time the trigger should fire at or after its configured start time,
	// using now as the reference clock. It returns the zero time if the
	// trigger will never fire.
	ComputeFirstFireTime(now time.Time) time.Time

	// NextFireTime returns the currently scheduled fire time, or the zero
	// time if the trigger is exhausted.
	NextFireTime() time.Time

	// PreviousFireTime returns the last time the trigger fired, or the zero
	// time if it has not fired yet.
	PreviousFireTime() time.Time

	// Triggered advances the trigger after a fire at the given time and
	// returns the next fire time (zero when exhausted).
	Triggered(now time.Time) time.Time

	// WillFireAgain reports whether NextFireTime is set.
	WillFireAgain() bool

	// MisfirePolicy returns the configured misfire policy.
	MisfirePolicy() MisfirePolicy

	// UpdateAfterMisfire recomputes the next fire time relative to now
	// according to the trigger's misfire policy and returns it.
	UpdateAfterMisfire(now time.Time) time.Time

	// Description returns an optional human readable description.
	Description() string
}
