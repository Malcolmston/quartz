package quartz

import (
	"fmt"
	"time"
)

// RepeatForever is used as the repeat count of a SimpleTrigger that should keep
// firing without a fixed limit.
const RepeatForever = -1

// SimpleTrigger fires starting at a given time, then repeats on a fixed
// interval either a bounded number of times or forever. A repeat count of N
// results in N+1 total fires (the initial fire plus N repeats), matching the
// semantics of the original Quartz project.
type SimpleTrigger struct {
	key           Key
	jobKey        Key
	description   string
	startTime     time.Time
	endTime       time.Time
	interval      time.Duration
	repeatCount   int
	misfirePolicy MisfirePolicy

	timesTriggered int
	next           time.Time
	prev           time.Time
}

// NewSimpleTrigger builds a SimpleTrigger. A repeatCount of RepeatForever means
// unbounded repeats. An interval of zero with a non-zero repeat count is
// invalid and will cause the trigger to fire only once.
func NewSimpleTrigger(key, jobKey Key, start time.Time, interval time.Duration, repeatCount int) *SimpleTrigger {
	return &SimpleTrigger{
		key:         key,
		jobKey:      jobKey,
		startTime:   start,
		interval:    interval,
		repeatCount: repeatCount,
	}
}

// WithDescription sets the trigger description and returns the trigger.
func (t *SimpleTrigger) WithDescription(desc string) *SimpleTrigger {
	t.description = desc
	return t
}

// WithEndTime sets a time after which the trigger will not fire and returns the
// trigger. The zero time means no end.
func (t *SimpleTrigger) WithEndTime(end time.Time) *SimpleTrigger {
	t.endTime = end
	return t
}

// WithMisfirePolicy sets the misfire policy and returns the trigger.
func (t *SimpleTrigger) WithMisfirePolicy(p MisfirePolicy) *SimpleTrigger {
	t.misfirePolicy = p
	return t
}

// Key implements Trigger.
func (t *SimpleTrigger) Key() Key { return t.key }

// JobKey implements Trigger.
func (t *SimpleTrigger) JobKey() Key { return t.jobKey }

// Description implements Trigger.
func (t *SimpleTrigger) Description() string { return t.description }

// MisfirePolicy implements Trigger.
func (t *SimpleTrigger) MisfirePolicy() MisfirePolicy { return t.misfirePolicy }

// StartTime returns the configured start time.
func (t *SimpleTrigger) StartTime() time.Time { return t.startTime }

// RepeatCount returns the configured repeat count.
func (t *SimpleTrigger) RepeatCount() int { return t.repeatCount }

// Interval returns the repeat interval.
func (t *SimpleTrigger) Interval() time.Duration { return t.interval }

// TimesTriggered returns the number of times the trigger has fired.
func (t *SimpleTrigger) TimesTriggered() int { return t.timesTriggered }

// ComputeFirstFireTime implements Trigger.
func (t *SimpleTrigger) ComputeFirstFireTime(now time.Time) time.Time {
	t.next = t.startTime
	if !t.endTime.IsZero() && t.next.After(t.endTime) {
		t.next = time.Time{}
	}
	return t.next
}

// NextFireTime implements Trigger.
func (t *SimpleTrigger) NextFireTime() time.Time { return t.next }

// PreviousFireTime implements Trigger.
func (t *SimpleTrigger) PreviousFireTime() time.Time { return t.prev }

// WillFireAgain implements Trigger.
func (t *SimpleTrigger) WillFireAgain() bool { return !t.next.IsZero() }

// Triggered implements Trigger.
func (t *SimpleTrigger) Triggered(now time.Time) time.Time {
	t.prev = t.next
	t.timesTriggered++
	t.next = t.computeAfter(t.next)
	return t.next
}

// computeAfter returns the fire time following the given fire time, honoring
// the repeat count and end time. It returns the zero time when exhausted.
func (t *SimpleTrigger) computeAfter(fired time.Time) time.Time {
	if t.repeatCount != RepeatForever && t.timesTriggered > t.repeatCount {
		return time.Time{}
	}
	if t.interval <= 0 {
		return time.Time{}
	}
	next := fired.Add(t.interval)
	if !t.endTime.IsZero() && next.After(t.endTime) {
		return time.Time{}
	}
	return next
}

// FireTimeAfter returns the first scheduled fire time strictly after the given
// time, or the zero time when the trigger has no such fire (it is exhausted by
// its repeat count or end time). It is a pure query that does not consult or
// mutate the trigger's running state, mirroring the original Quartz
// SimpleTrigger.getFireTimeAfter(Date). Fire times are the start time plus
// whole multiples of the interval; a trigger with RepeatForever is unbounded,
// otherwise it fires repeatCount+1 times in total.
func (t *SimpleTrigger) FireTimeAfter(after time.Time) time.Time {
	if t.startTime.After(after) {
		if !t.endTime.IsZero() && t.startTime.After(t.endTime) {
			return time.Time{}
		}
		return t.startTime
	}
	if t.interval <= 0 {
		// The only fire is the start time, which is not after 'after'.
		return time.Time{}
	}
	n := int(after.Sub(t.startTime)/t.interval) + 1
	if t.repeatCount != RepeatForever && n > t.repeatCount {
		return time.Time{}
	}
	fire := t.startTime.Add(time.Duration(n) * t.interval)
	if !t.endTime.IsZero() && fire.After(t.endTime) {
		return time.Time{}
	}
	return fire
}

// UpdateAfterMisfire implements Trigger. It reschedules the trigger relative to
// now according to its misfire policy.
func (t *SimpleTrigger) UpdateAfterMisfire(now time.Time) time.Time {
	switch t.misfirePolicy {
	case MisfireIgnore:
		// Leave next as-is; the scheduler will fire rapidly to catch up.
		return t.next
	case MisfireFireNow:
		if t.repeatCount == RepeatForever || t.timesTriggered <= t.repeatCount {
			t.next = now
		}
		return t.next
	default: // MisfireSmart and MisfireDoNothing advance past now.
		if t.interval <= 0 {
			if t.next.Before(now) {
				t.next = time.Time{}
			}
			return t.next
		}
		// Advance next forward past now while consuming repeats.
		for !t.next.IsZero() && !t.next.After(now) {
			t.timesTriggered++
			t.next = t.computeAfter(t.next)
		}
		return t.next
	}
}

// String returns a debug representation of the trigger.
func (t *SimpleTrigger) String() string {
	return fmt.Sprintf("SimpleTrigger{key=%s, job=%s, interval=%s, repeat=%d}",
		t.key, t.jobKey, t.interval, t.repeatCount)
}
