package quartz

import (
	"fmt"
	"time"
)

// CronTrigger fires according to a CronExpression, evaluated in a configurable
// time zone. Fire times are never before the trigger's start time nor after its
// optional end time.
type CronTrigger struct {
	key           Key
	jobKey        Key
	description   string
	expr          *CronExpression
	location      *time.Location
	startTime     time.Time
	endTime       time.Time
	misfirePolicy MisfirePolicy

	next time.Time
	prev time.Time
}

// NewCronTrigger builds a CronTrigger from a cron expression string. The
// expression is evaluated in UTC unless changed with In. It returns an error if
// the expression is invalid.
func NewCronTrigger(key, jobKey, expr string) (*CronTrigger, error) {
	ce, err := ParseCron(expr)
	if err != nil {
		return nil, err
	}
	return &CronTrigger{
		key:      NewKey(key),
		jobKey:   NewKey(jobKey),
		expr:     ce,
		location: time.UTC,
	}, nil
}

// NewCronTriggerWithKeys is like NewCronTrigger but takes fully qualified keys.
func NewCronTriggerWithKeys(key, jobKey Key, expr string) (*CronTrigger, error) {
	ce, err := ParseCron(expr)
	if err != nil {
		return nil, err
	}
	return &CronTrigger{
		key:      key,
		jobKey:   jobKey,
		expr:     ce,
		location: time.UTC,
	}, nil
}

// In sets the time zone used to evaluate the cron expression and returns the
// trigger. A nil location is ignored.
func (t *CronTrigger) In(loc *time.Location) *CronTrigger {
	if loc != nil {
		t.location = loc
	}
	return t
}

// WithDescription sets the trigger description and returns the trigger.
func (t *CronTrigger) WithDescription(desc string) *CronTrigger {
	t.description = desc
	return t
}

// StartingAt sets the earliest time the trigger may fire and returns it.
func (t *CronTrigger) StartingAt(start time.Time) *CronTrigger {
	t.startTime = start
	return t
}

// EndingAt sets the latest time the trigger may fire and returns it.
func (t *CronTrigger) EndingAt(end time.Time) *CronTrigger {
	t.endTime = end
	return t
}

// WithMisfirePolicy sets the misfire policy and returns the trigger.
func (t *CronTrigger) WithMisfirePolicy(p MisfirePolicy) *CronTrigger {
	t.misfirePolicy = p
	return t
}

// Expression returns the parsed cron expression.
func (t *CronTrigger) Expression() *CronExpression { return t.expr }

// Key implements Trigger.
func (t *CronTrigger) Key() Key { return t.key }

// JobKey implements Trigger.
func (t *CronTrigger) JobKey() Key { return t.jobKey }

// Description implements Trigger.
func (t *CronTrigger) Description() string { return t.description }

// MisfirePolicy implements Trigger.
func (t *CronTrigger) MisfirePolicy() MisfirePolicy { return t.misfirePolicy }

// ComputeFirstFireTime implements Trigger.
func (t *CronTrigger) ComputeFirstFireTime(now time.Time) time.Time {
	after := now
	if !t.startTime.IsZero() && t.startTime.After(now) {
		// Allow the start time itself to be a valid fire time.
		after = t.startTime.Add(-time.Second)
	}
	t.next = t.afterInLocation(after)
	return t.next
}

// afterInLocation computes the next cron fire time strictly after the given
// instant, evaluated in the trigger's location, honoring the end time.
func (t *CronTrigger) afterInLocation(after time.Time) time.Time {
	n := t.expr.Next(after.In(t.location))
	if n.IsZero() {
		return time.Time{}
	}
	if !t.endTime.IsZero() && n.After(t.endTime) {
		return time.Time{}
	}
	return n
}

// NextFireTime implements Trigger.
func (t *CronTrigger) NextFireTime() time.Time { return t.next }

// PreviousFireTime implements Trigger.
func (t *CronTrigger) PreviousFireTime() time.Time { return t.prev }

// WillFireAgain implements Trigger.
func (t *CronTrigger) WillFireAgain() bool { return !t.next.IsZero() }

// Triggered implements Trigger.
func (t *CronTrigger) Triggered(now time.Time) time.Time {
	t.prev = t.next
	base := t.next
	if base.IsZero() {
		base = now
	}
	t.next = t.afterInLocation(base)
	return t.next
}

// UpdateAfterMisfire implements Trigger.
func (t *CronTrigger) UpdateAfterMisfire(now time.Time) time.Time {
	switch t.misfirePolicy {
	case MisfireIgnore:
		return t.next
	case MisfireFireNow:
		t.next = now
		return t.next
	default: // Smart / DoNothing: advance to the next valid time after now.
		t.next = t.afterInLocation(now)
		return t.next
	}
}

// String returns a debug representation of the trigger.
func (t *CronTrigger) String() string {
	return fmt.Sprintf("CronTrigger{key=%s, job=%s, expr=%q, tz=%s}",
		t.key, t.jobKey, t.expr, t.location)
}
