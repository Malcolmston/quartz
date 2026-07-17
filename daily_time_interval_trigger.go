package quartz

import (
	"fmt"
	"time"
)

// daily_time_interval_triggerMaxDayScan bounds how many consecutive calendar
// days the slot search will examine before giving up. It is large enough to
// always cross into the next active weekday (at most seven days away) with room
// to spare, yet finite so a trigger with no active days or an empty firing
// window cannot loop forever.
const daily_time_interval_triggerMaxDayScan = 400

// TimeOfDay is a wall-clock time-of-day, expressed as an hour, minute and
// second offset from midnight. It carries no date or location; a
// DailyTimeIntervalTrigger interprets it against each active day in the
// trigger's own location so that window boundaries are wall-clock times.
type TimeOfDay struct {
	// Hour is the hour of the day in the range [0, 23].
	Hour int
	// Minute is the minute of the hour in the range [0, 59].
	Minute int
	// Second is the second of the minute in the range [0, 59].
	Second int
}

// NewTimeOfDay builds a TimeOfDay from an hour, minute and second. The values
// are stored as given; callers are responsible for supplying sane, normalized
// components (for example 0-23 for the hour).
func NewTimeOfDay(h, m, s int) TimeOfDay {
	return TimeOfDay{Hour: h, Minute: m, Second: s}
}

// daily_time_interval_triggerDurationOf converts a TimeOfDay into a duration
// offset from midnight.
func daily_time_interval_triggerDurationOf(tod TimeOfDay) time.Duration {
	return time.Duration(tod.Hour)*time.Hour +
		time.Duration(tod.Minute)*time.Minute +
		time.Duration(tod.Second)*time.Second
}

// daily_time_interval_triggerTimeOfDayOf converts a duration offset from
// midnight back into a TimeOfDay.
func daily_time_interval_triggerTimeOfDayOf(d time.Duration) TimeOfDay {
	total := int(d / time.Second)
	return TimeOfDay{
		Hour:   total / 3600,
		Minute: (total % 3600) / 60,
		Second: total % 60,
	}
}

// daily_time_interval_triggerAllWeekdays returns a fresh slice containing all
// seven weekdays, Sunday through Saturday.
func daily_time_interval_triggerAllWeekdays() []time.Weekday {
	return []time.Weekday{
		time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday,
	}
}

// DailyTimeIntervalTrigger fires on a fixed sub-day interval within a daily
// time-of-day window, but only on a selected set of weekdays. Within each
// active day it starts at the window's start time-of-day and steps forward by
// count units (seconds, minutes or hours) as long as the next step does not
// pass the window's end time-of-day; once it would pass the end it rolls to the
// start time-of-day of the next active day. All arithmetic is performed in the
// trigger's location (time.UTC by default) so the window boundaries are
// wall-clock times that stay fixed across daylight saving transitions.
//
// The trigger reuses IntervalUnit from CalendarIntervalTrigger (restricted to
// IntervalSecond, IntervalMinute and IntervalHour) and the RepeatForever
// sentinel from SimpleTrigger. A repeat count of N produces N+1 total fires;
// RepeatForever fires without a fixed limit. Its startTime and endTime bound
// the trigger absolutely, independent of the daily window.
//
// Its next, prev and timesTriggered state fields mirror SimpleTrigger and
// CalendarIntervalTrigger so a SQL job store can persist and restore an
// in-flight trigger.
type DailyTimeIntervalTrigger struct {
	key            Key
	jobKey         Key
	description    string
	startTime      time.Time
	endTime        time.Time
	startTimeOfDay time.Duration
	endTimeOfDay   time.Duration
	daysOfWeek     []time.Weekday
	interval       int
	unit           IntervalUnit
	repeatCount    int
	location       *time.Location
	misfirePolicy  MisfirePolicy

	timesTriggered int
	next           time.Time
	prev           time.Time
}

// NewDailyTimeIntervalTrigger builds a DailyTimeIntervalTrigger whose daily
// firing window runs from start to end (both wall-clock times-of-day), stepping
// by count of the given unit. The unit should be one of IntervalSecond,
// IntervalMinute or IntervalHour; other units yield a zero step and the trigger
// then fires once per active day at the window start. By default the trigger is
// active on all seven weekdays, evaluates times in time.UTC, and repeats
// forever; use the fluent builders to change these. The window must have end
// no earlier than start, otherwise the trigger never fires.
func NewDailyTimeIntervalTrigger(key, jobKey Key, start TimeOfDay, end TimeOfDay, unit IntervalUnit, count int) *DailyTimeIntervalTrigger {
	return &DailyTimeIntervalTrigger{
		key:            key,
		jobKey:         jobKey,
		startTimeOfDay: daily_time_interval_triggerDurationOf(start),
		endTimeOfDay:   daily_time_interval_triggerDurationOf(end),
		daysOfWeek:     daily_time_interval_triggerAllWeekdays(),
		interval:       count,
		unit:           unit,
		repeatCount:    RepeatForever,
		location:       time.UTC,
	}
}

// OnDaysOfWeek restricts the trigger to fire only on the given weekdays and
// returns the trigger. Calling it with no arguments leaves the current set
// unchanged (the default is all seven weekdays). The supplied slice is copied
// so later external mutation does not leak in.
func (t *DailyTimeIntervalTrigger) OnDaysOfWeek(days ...time.Weekday) *DailyTimeIntervalTrigger {
	if len(days) == 0 {
		return t
	}
	t.daysOfWeek = make([]time.Weekday, len(days))
	copy(t.daysOfWeek, days)
	return t
}

// In sets the time zone used to evaluate the daily window and returns the
// trigger. A nil location is ignored, leaving the current location in place.
func (t *DailyTimeIntervalTrigger) In(loc *time.Location) *DailyTimeIntervalTrigger {
	if loc != nil {
		t.location = loc
	}
	return t
}

// StartingAt sets the absolute time before which the trigger will not fire and
// returns the trigger. The zero time means the trigger has no lower bound
// beyond the reference clock passed to ComputeFirstFireTime.
func (t *DailyTimeIntervalTrigger) StartingAt(start time.Time) *DailyTimeIntervalTrigger {
	t.startTime = start
	return t
}

// EndingAt sets the absolute time after which the trigger will not fire and
// returns the trigger. The zero time means the trigger has no end.
func (t *DailyTimeIntervalTrigger) EndingAt(end time.Time) *DailyTimeIntervalTrigger {
	t.endTime = end
	return t
}

// WithRepeatCount sets the number of repeats after the initial fire and returns
// the trigger. A count of N yields N+1 total fires; RepeatForever fires without
// a fixed limit.
func (t *DailyTimeIntervalTrigger) WithRepeatCount(count int) *DailyTimeIntervalTrigger {
	t.repeatCount = count
	return t
}

// WithDescription sets the trigger description and returns the trigger.
func (t *DailyTimeIntervalTrigger) WithDescription(desc string) *DailyTimeIntervalTrigger {
	t.description = desc
	return t
}

// WithMisfirePolicy sets the misfire policy and returns the trigger.
func (t *DailyTimeIntervalTrigger) WithMisfirePolicy(p MisfirePolicy) *DailyTimeIntervalTrigger {
	t.misfirePolicy = p
	return t
}

// StartTimeOfDay returns the start of the daily firing window.
func (t *DailyTimeIntervalTrigger) StartTimeOfDay() TimeOfDay {
	return daily_time_interval_triggerTimeOfDayOf(t.startTimeOfDay)
}

// EndTimeOfDay returns the end of the daily firing window.
func (t *DailyTimeIntervalTrigger) EndTimeOfDay() TimeOfDay {
	return daily_time_interval_triggerTimeOfDayOf(t.endTimeOfDay)
}

// DaysOfWeek returns a copy of the weekdays on which the trigger is active.
func (t *DailyTimeIntervalTrigger) DaysOfWeek() []time.Weekday {
	out := make([]time.Weekday, len(t.daysOfWeek))
	copy(out, t.daysOfWeek)
	return out
}

// StartTime returns the configured absolute start time, or the zero time when
// unbounded.
func (t *DailyTimeIntervalTrigger) StartTime() time.Time { return t.startTime }

// EndTime returns the configured absolute end time, or the zero time when
// unbounded.
func (t *DailyTimeIntervalTrigger) EndTime() time.Time { return t.endTime }

// Unit returns the sub-day interval unit the trigger advances by within a day.
func (t *DailyTimeIntervalTrigger) Unit() IntervalUnit { return t.unit }

// Count returns the number of units advanced between fires within a day.
func (t *DailyTimeIntervalTrigger) Count() int { return t.interval }

// RepeatCount returns the configured repeat count.
func (t *DailyTimeIntervalTrigger) RepeatCount() int { return t.repeatCount }

// Location returns the time zone used to evaluate the daily window.
func (t *DailyTimeIntervalTrigger) Location() *time.Location { return t.location }

// TimesTriggered returns the number of times the trigger has fired.
func (t *DailyTimeIntervalTrigger) TimesTriggered() int { return t.timesTriggered }

// Key implements Trigger.
func (t *DailyTimeIntervalTrigger) Key() Key { return t.key }

// JobKey implements Trigger.
func (t *DailyTimeIntervalTrigger) JobKey() Key { return t.jobKey }

// Description implements Trigger.
func (t *DailyTimeIntervalTrigger) Description() string { return t.description }

// MisfirePolicy implements Trigger.
func (t *DailyTimeIntervalTrigger) MisfirePolicy() MisfirePolicy { return t.misfirePolicy }

// NextFireTime implements Trigger.
func (t *DailyTimeIntervalTrigger) NextFireTime() time.Time { return t.next }

// PreviousFireTime implements Trigger.
func (t *DailyTimeIntervalTrigger) PreviousFireTime() time.Time { return t.prev }

// WillFireAgain implements Trigger.
func (t *DailyTimeIntervalTrigger) WillFireAgain() bool { return !t.next.IsZero() }

// ComputeFirstFireTime implements Trigger. It finds the first in-window slot at
// or after the later of the reference clock now and the configured start time,
// honoring the absolute end time. It returns the zero time when the trigger
// will never fire.
func (t *DailyTimeIntervalTrigger) ComputeFirstFireTime(now time.Time) time.Time {
	lower := now
	if !t.startTime.IsZero() && t.startTime.After(lower) {
		lower = t.startTime
	}
	t.next = t.daily_time_interval_triggerNextSlot(lower, false)
	if !t.next.IsZero() && !t.endTime.IsZero() && t.next.After(t.endTime) {
		t.next = time.Time{}
	}
	return t.next
}

// Triggered implements Trigger. It records the just-fired time, advances to the
// next in-window slot (or the start of the next active day), and returns the
// new next fire time (the zero time once the trigger is exhausted by its repeat
// count or end time).
func (t *DailyTimeIntervalTrigger) Triggered(now time.Time) time.Time {
	t.prev = t.next
	t.timesTriggered++
	t.next = t.daily_time_interval_triggerComputeAfter(t.next)
	return t.next
}

// UpdateAfterMisfire implements Trigger. It reschedules the trigger relative to
// now according to its misfire policy, using the same dispatch as the other
// triggers: MisfireIgnore leaves the next fire time unchanged, MisfireFireNow
// sets it to now (while repeats remain), and MisfireSmart and MisfireDoNothing
// advance slot by slot until the next fire time is strictly after now.
func (t *DailyTimeIntervalTrigger) UpdateAfterMisfire(now time.Time) time.Time {
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
		for !t.next.IsZero() && !t.next.After(now) {
			t.timesTriggered++
			t.next = t.daily_time_interval_triggerComputeAfter(t.next)
		}
		return t.next
	}
}

// daily_time_interval_triggerComputeAfter returns the first in-window slot
// strictly after the given fire time, honoring the repeat count and absolute
// end time. It returns the zero time when the trigger is exhausted.
func (t *DailyTimeIntervalTrigger) daily_time_interval_triggerComputeAfter(fired time.Time) time.Time {
	if fired.IsZero() {
		return time.Time{}
	}
	if t.repeatCount != RepeatForever && t.timesTriggered > t.repeatCount {
		return time.Time{}
	}
	next := t.daily_time_interval_triggerNextSlot(fired, true)
	if next.IsZero() {
		return time.Time{}
	}
	if !t.endTime.IsZero() && next.After(t.endTime) {
		return time.Time{}
	}
	return next
}

// daily_time_interval_triggerStep returns the intra-day advancement as an
// absolute duration. A non-positive count or a non sub-day unit yields a zero
// step, in which case the trigger fires only at the window start of each active
// day.
func (t *DailyTimeIntervalTrigger) daily_time_interval_triggerStep() time.Duration {
	if t.interval <= 0 {
		return 0
	}
	switch t.unit {
	case IntervalSecond:
		return time.Duration(t.interval) * time.Second
	case IntervalMinute:
		return time.Duration(t.interval) * time.Minute
	case IntervalHour:
		return time.Duration(t.interval) * time.Hour
	default:
		return 0
	}
}

// daily_time_interval_triggerAllowed returns a lookup table of which weekdays
// are active.
func (t *DailyTimeIntervalTrigger) daily_time_interval_triggerAllowed() [7]bool {
	var set [7]bool
	for _, d := range t.daysOfWeek {
		if d >= time.Sunday && d <= time.Saturday {
			set[d] = true
		}
	}
	return set
}

// daily_time_interval_triggerAt builds the wall-clock instant for the given
// calendar date and time-of-day offset in the trigger's location. Day overflow
// is normalized by time.Date, and the wall-clock components are preserved
// across daylight saving transitions.
func (t *DailyTimeIntervalTrigger) daily_time_interval_triggerAt(year int, month time.Month, day int, tod time.Duration, loc *time.Location) time.Time {
	total := int(tod / time.Second)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return time.Date(year, month, day, h, m, s, 0, loc)
}

// daily_time_interval_triggerNextSlot returns the first firing slot relative to
// the lower bound. When strict is true the slot must be strictly after lower
// (used when advancing past a fire); otherwise the slot may equal lower (used
// when computing the first fire). It scans forward day by day, and within each
// active day steps from the window start by the intra-day step while the offset
// does not pass the window end. It returns the zero time when no slot is found
// within the bounded scan (for example when no weekday is active or the window
// is empty).
func (t *DailyTimeIntervalTrigger) daily_time_interval_triggerNextSlot(lower time.Time, strict bool) time.Time {
	loc := t.location
	if loc == nil {
		loc = time.UTC
	}
	allowed := t.daily_time_interval_triggerAllowed()
	step := t.daily_time_interval_triggerStep()
	startTOD := t.startTimeOfDay
	endTOD := t.endTimeOfDay

	l := lower.In(loc)
	year, month, day := l.Date()

	for i := 0; i < daily_time_interval_triggerMaxDayScan; i++ {
		dayMidnight := time.Date(year, month, day+i, 0, 0, 0, 0, loc)
		if !allowed[dayMidnight.Weekday()] {
			continue
		}
		for tod := startTOD; tod <= endTOD; {
			slot := t.daily_time_interval_triggerAt(year, month, day+i, tod, loc)
			var ok bool
			if strict {
				ok = slot.After(lower)
			} else {
				ok = !slot.Before(lower)
			}
			if ok {
				return slot
			}
			if step <= 0 {
				break
			}
			tod += step
		}
	}
	return time.Time{}
}

// String returns a debug representation of the trigger.
func (t *DailyTimeIntervalTrigger) String() string {
	start := daily_time_interval_triggerTimeOfDayOf(t.startTimeOfDay)
	end := daily_time_interval_triggerTimeOfDayOf(t.endTimeOfDay)
	return fmt.Sprintf("DailyTimeIntervalTrigger{key=%s, job=%s, window=%02d:%02d:%02d-%02d:%02d:%02d, unit=%d, count=%d, tz=%s}",
		t.key, t.jobKey,
		start.Hour, start.Minute, start.Second,
		end.Hour, end.Minute, end.Second,
		t.unit, t.interval, t.location)
}
