package quartz

import (
	"fmt"
	"time"
)

// IntervalUnit identifies the calendar unit by which a CalendarIntervalTrigger
// advances between fires. Unlike a fixed time.Duration, calendar units honor the
// varying length of months and years as well as daylight saving transitions.
type IntervalUnit int

const (
	// IntervalSecond advances by a fixed number of seconds. It is a sub-day
	// unit and is added as an absolute duration.
	IntervalSecond IntervalUnit = iota
	// IntervalMinute advances by a fixed number of minutes. It is a sub-day
	// unit and is added as an absolute duration.
	IntervalMinute
	// IntervalHour advances by a fixed number of hours. It is a sub-day unit
	// and is added as an absolute duration.
	IntervalHour
	// IntervalDay advances by whole calendar days, evaluated in the trigger's
	// location so wall-clock time is preserved across daylight saving changes.
	IntervalDay
	// IntervalWeek advances by whole calendar weeks (seven days per count),
	// evaluated in the trigger's location.
	IntervalWeek
	// IntervalMonth advances by whole calendar months, honoring the differing
	// number of days per month.
	IntervalMonth
	// IntervalYear advances by whole calendar years, honoring leap years.
	IntervalYear
)

// CalendarIntervalTrigger fires starting at a given time and then repeats at a
// fixed count of calendar units. Because it advances by calendar units rather
// than a fixed duration, successive fires honor the varying length of months and
// years and remain aligned to wall-clock time across daylight saving
// transitions. Advancement is computed in the trigger's location (time.UTC by
// default): day, week, month and year steps use time.Time.AddDate while
// sub-day steps (second, minute, hour) are added as an absolute duration with
// time.Time.Add.
//
// Its next, prev and timesTriggered state fields mirror SimpleTrigger so a SQL
// job store can persist and restore an in-flight trigger.
type CalendarIntervalTrigger struct {
	key               Key
	jobKey            Key
	description       string
	startTime         time.Time
	endTime           time.Time
	unit              IntervalUnit
	count             int
	location          *time.Location
	misfirePolicy     MisfirePolicy
	preserveWallClock bool

	timesTriggered int
	next           time.Time
	prev           time.Time
}

// NewCalendarIntervalTrigger builds a CalendarIntervalTrigger that first fires at
// start and then repeats every count units. A count that is zero or negative
// yields a trigger that fires only once. The trigger evaluates advancement in
// time.UTC until changed with In.
func NewCalendarIntervalTrigger(key, jobKey Key, start time.Time, unit IntervalUnit, count int) *CalendarIntervalTrigger {
	return &CalendarIntervalTrigger{
		key:       key,
		jobKey:    jobKey,
		startTime: start,
		unit:      unit,
		count:     count,
		location:  time.UTC,
	}
}

// In sets the time zone used to evaluate calendar advancement and returns the
// trigger. A nil location is ignored, leaving the current location in place.
func (t *CalendarIntervalTrigger) In(loc *time.Location) *CalendarIntervalTrigger {
	if loc != nil {
		t.location = loc
	}
	return t
}

// WithEndTime sets a time after which the trigger will not fire and returns the
// trigger. The zero time means the trigger has no end.
func (t *CalendarIntervalTrigger) WithEndTime(end time.Time) *CalendarIntervalTrigger {
	t.endTime = end
	return t
}

// WithDescription sets the trigger description and returns the trigger.
func (t *CalendarIntervalTrigger) WithDescription(desc string) *CalendarIntervalTrigger {
	t.description = desc
	return t
}

// WithMisfirePolicy sets the misfire policy and returns the trigger.
func (t *CalendarIntervalTrigger) WithMisfirePolicy(p MisfirePolicy) *CalendarIntervalTrigger {
	t.misfirePolicy = p
	return t
}

// PreserveWallClock controls how day, week, month and year advancement behaves
// across daylight saving transitions and returns the trigger. When enabled the
// natural time.Time.AddDate result is used, which keeps the wall-clock
// time-of-day fixed (for example a 09:00 local fire stays at 09:00 local even
// when the UTC offset changes). When disabled the result is shifted by the
// change in UTC offset so the absolute time-of-day (in UTC) is preserved
// instead. It has no effect on sub-day units or in zones without daylight
// saving. It is disabled by default.
func (t *CalendarIntervalTrigger) PreserveWallClock(preserve bool) *CalendarIntervalTrigger {
	t.preserveWallClock = preserve
	return t
}

// StartTime returns the configured start time.
func (t *CalendarIntervalTrigger) StartTime() time.Time { return t.startTime }

// EndTime returns the configured end time, or the zero time when unbounded.
func (t *CalendarIntervalTrigger) EndTime() time.Time { return t.endTime }

// Unit returns the calendar unit the trigger advances by.
func (t *CalendarIntervalTrigger) Unit() IntervalUnit { return t.unit }

// Count returns the number of units advanced between fires.
func (t *CalendarIntervalTrigger) Count() int { return t.count }

// Location returns the time zone used to evaluate calendar advancement.
func (t *CalendarIntervalTrigger) Location() *time.Location { return t.location }

// TimesTriggered returns the number of times the trigger has fired.
func (t *CalendarIntervalTrigger) TimesTriggered() int { return t.timesTriggered }

// Key implements Trigger.
func (t *CalendarIntervalTrigger) Key() Key { return t.key }

// JobKey implements Trigger.
func (t *CalendarIntervalTrigger) JobKey() Key { return t.jobKey }

// Description implements Trigger.
func (t *CalendarIntervalTrigger) Description() string { return t.description }

// MisfirePolicy implements Trigger.
func (t *CalendarIntervalTrigger) MisfirePolicy() MisfirePolicy { return t.misfirePolicy }

// ComputeFirstFireTime implements Trigger. It sets the first fire time to the
// configured start time, or the zero time when the start time is already past
// the end time. The now argument is unused because the first fire is always the
// start time.
func (t *CalendarIntervalTrigger) ComputeFirstFireTime(now time.Time) time.Time {
	t.next = t.startTime
	if !t.endTime.IsZero() && t.next.After(t.endTime) {
		t.next = time.Time{}
	}
	return t.next
}

// NextFireTime implements Trigger.
func (t *CalendarIntervalTrigger) NextFireTime() time.Time { return t.next }

// PreviousFireTime implements Trigger.
func (t *CalendarIntervalTrigger) PreviousFireTime() time.Time { return t.prev }

// WillFireAgain implements Trigger.
func (t *CalendarIntervalTrigger) WillFireAgain() bool { return !t.next.IsZero() }

// Triggered implements Trigger. It records the just-fired time, advances the
// next fire time by one unit*count step, and returns the new next fire time
// (the zero time once the trigger is exhausted by its end time or a
// non-positive count).
func (t *CalendarIntervalTrigger) Triggered(now time.Time) time.Time {
	t.prev = t.next
	t.timesTriggered++
	t.next = t.calendar_interval_triggerComputeAfter(t.next)
	return t.next
}

// UpdateAfterMisfire implements Trigger. It reschedules the trigger relative to
// now according to its misfire policy: MisfireIgnore leaves the next fire time
// unchanged, MisfireFireNow sets it to now, and MisfireSmart and
// MisfireDoNothing repeatedly advance whole unit*count steps until the next
// fire time is strictly after now.
func (t *CalendarIntervalTrigger) UpdateAfterMisfire(now time.Time) time.Time {
	switch t.misfirePolicy {
	case MisfireIgnore:
		// Leave next as-is; the scheduler will fire rapidly to catch up.
		return t.next
	case MisfireFireNow:
		t.next = now
		return t.next
	default: // MisfireSmart and MisfireDoNothing advance past now.
		for !t.next.IsZero() && !t.next.After(now) {
			t.next = t.calendar_interval_triggerAdvance(t.next)
			if !t.next.IsZero() && !t.endTime.IsZero() && t.next.After(t.endTime) {
				t.next = time.Time{}
			}
		}
		return t.next
	}
}

// calendar_interval_triggerComputeAfter returns the fire time following the
// given fire time, honoring the end time. It returns the zero time when the
// trigger is exhausted.
func (t *CalendarIntervalTrigger) calendar_interval_triggerComputeAfter(fired time.Time) time.Time {
	if fired.IsZero() {
		return time.Time{}
	}
	next := t.calendar_interval_triggerAdvance(fired)
	if next.IsZero() {
		return time.Time{}
	}
	if !t.endTime.IsZero() && next.After(t.endTime) {
		return time.Time{}
	}
	return next
}

// calendar_interval_triggerAdvance advances the given time by one unit*count
// step, evaluated in the trigger's location. Day, week, month and year steps use
// time.Time.AddDate; second, minute and hour steps are added as an absolute
// duration. A non-positive count returns the zero time so callers do not loop
// forever on a trigger that cannot make progress.
func (t *CalendarIntervalTrigger) calendar_interval_triggerAdvance(fired time.Time) time.Time {
	if t.count <= 0 {
		return time.Time{}
	}
	loc := t.location
	if loc == nil {
		loc = time.UTC
	}
	base := fired.In(loc)
	switch t.unit {
	case IntervalSecond:
		return base.Add(time.Duration(t.count) * time.Second)
	case IntervalMinute:
		return base.Add(time.Duration(t.count) * time.Minute)
	case IntervalHour:
		return base.Add(time.Duration(t.count) * time.Hour)
	case IntervalDay:
		return t.calendar_interval_triggerDateAdvance(base, 0, 0, t.count)
	case IntervalWeek:
		return t.calendar_interval_triggerDateAdvance(base, 0, 0, 7*t.count)
	case IntervalMonth:
		return t.calendar_interval_triggerDateAdvance(base, 0, t.count, 0)
	case IntervalYear:
		return t.calendar_interval_triggerDateAdvance(base, t.count, 0, 0)
	default:
		return time.Time{}
	}
}

// calendar_interval_triggerDateAdvance applies a calendar (AddDate) advancement
// to base. When the trigger does not preserve wall-clock time, the result is
// shifted by any change in UTC offset so the absolute time-of-day is kept fixed
// across a daylight saving transition instead of the local wall-clock time.
func (t *CalendarIntervalTrigger) calendar_interval_triggerDateAdvance(base time.Time, years, months, days int) time.Time {
	adv := base.AddDate(years, months, days)
	if !t.preserveWallClock {
		_, baseOff := base.Zone()
		_, advOff := adv.Zone()
		adv = adv.Add(time.Duration(advOff-baseOff) * time.Second)
	}
	return adv
}

// String returns a debug representation of the trigger.
func (t *CalendarIntervalTrigger) String() string {
	return fmt.Sprintf("CalendarIntervalTrigger{key=%s, job=%s, unit=%d, count=%d, tz=%s}",
		t.key, t.jobKey, t.unit, t.count, t.location)
}
