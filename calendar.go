package quartz

import "time"

// calendarMaxIterations bounds the forward scan performed by a calendar's
// NextIncludedTime so a pathological calendar (for example a cron expression
// that matches every instant) cannot loop forever. Day-granularity calendars
// advance a whole day per iteration, so the bound is never approached in
// practice; it exists only as a safety net.
const calendarMaxIterations = 1 << 20

// calendarMaxTriggerAdvances bounds how many times a CalendarTrigger will step
// its inner trigger while skipping excluded fire times. It guards against an
// inner trigger that never makes forward progress or a calendar that excludes
// an unbounded run of fire times.
const calendarMaxTriggerAdvances = 1 << 16

// Calendar excludes selected instants from a trigger's fire schedule. Calendars
// are additive: a Calendar may wrap a base Calendar so that an instant is
// considered included only when it is included by this Calendar and by its base,
// letting several exclusion rules compose into one. Implementations must be safe
// for use by a single scheduler goroutine at a time, matching the concurrency
// contract of Trigger.
type Calendar interface {
	// IsTimeIncluded reports whether t is allowed to fire, that is whether it
	// is excluded neither by this Calendar nor by any base Calendar.
	IsTimeIncluded(t time.Time) bool
	// NextIncludedTime returns the earliest instant at or after t that
	// IsTimeIncluded reports as included. When t itself is included it is
	// returned unchanged.
	NextIncludedTime(t time.Time) time.Time
	// SetBaseCalendar sets the base Calendar consulted in addition to this
	// Calendar. A nil base clears any existing base.
	SetBaseCalendar(Calendar)
	// GetBaseCalendar returns the base Calendar, or nil when none is set.
	GetBaseCalendar() Calendar
}

// BaseCalendar provides the state and delegation shared by every Calendar
// implementation in this package. It stores an optional base Calendar and an
// optional evaluation location, and is meant to be embedded. A BaseCalendar
// excludes no instant of its own, so used directly it behaves as a pass-through
// to its base (or includes everything when it has no base). Embedding types
// override IsTimeIncluded and NextIncludedTime to add their own exclusion rule
// while still consulting the base through the helpers it provides.
type BaseCalendar struct {
	baseCalendar Calendar
	location     *time.Location
}

// SetBaseCalendar sets the base Calendar consulted in addition to this one. A
// nil base clears any existing base.
func (b *BaseCalendar) SetBaseCalendar(base Calendar) { b.baseCalendar = base }

// GetBaseCalendar returns the base Calendar, or nil when none is set.
func (b *BaseCalendar) GetBaseCalendar() Calendar { return b.baseCalendar }

// SetLocation sets the time zone in which wall-clock exclusions (day, weekday,
// time-of-day and cron rules) are evaluated. A nil location means each instant
// is evaluated in its own location.
func (b *BaseCalendar) SetLocation(loc *time.Location) { b.location = loc }

// Location returns the evaluation time zone, or nil when instants are evaluated
// in their own location.
func (b *BaseCalendar) Location() *time.Location { return b.location }

// IsTimeIncluded reports whether t is included. A BaseCalendar contributes no
// exclusion of its own, so the result is determined solely by its base Calendar
// (or true when there is no base).
func (b *BaseCalendar) IsTimeIncluded(t time.Time) bool { return b.calendarBaseIncluded(t) }

// NextIncludedTime returns the next instant at or after t allowed by the base
// Calendar, or t itself when there is no base.
func (b *BaseCalendar) NextIncludedTime(t time.Time) time.Time {
	return b.calendarBaseNextIncludedTime(t)
}

// calendarBaseIncluded reports whether the base Calendar includes t, treating a
// missing base as including every instant.
func (b *BaseCalendar) calendarBaseIncluded(t time.Time) bool {
	return b.baseCalendar == nil || b.baseCalendar.IsTimeIncluded(t)
}

// calendarBaseNextIncludedTime returns the base Calendar's next included instant
// at or after t, or t itself when there is no base.
func (b *BaseCalendar) calendarBaseNextIncludedTime(t time.Time) time.Time {
	if b.baseCalendar == nil {
		return t
	}
	return b.baseCalendar.NextIncludedTime(t)
}

// calendarEval returns t expressed in the calendar's evaluation location, or t
// unchanged when no location is configured.
func (b *BaseCalendar) calendarEval(t time.Time) time.Time {
	if b.location != nil {
		return t.In(b.location)
	}
	return t
}

// calendarNextIncluded scans forward from t and returns the earliest instant
// that the embedding calendar's own rule (selfIncluded) allows and that the base
// Calendar also allows. When the own rule excludes an instant, advance computes
// the next instant to test, letting each calendar skip an excluded region in one
// step; when only the base excludes an instant, the base's NextIncludedTime is
// used to skip ahead.
func calendarNextIncluded(b *BaseCalendar, t time.Time, selfIncluded func(time.Time) bool, advance func(time.Time) time.Time) time.Time {
	next := t
	for i := 0; i < calendarMaxIterations; i++ {
		if !selfIncluded(next) {
			next = advance(next)
			continue
		}
		if b.calendarBaseIncluded(next) {
			return next
		}
		next = b.calendarBaseNextIncludedTime(next)
	}
	return next
}

// calendarSecondOfDay returns the number of seconds elapsed since midnight for t
// in its own location.
func calendarSecondOfDay(t time.Time) int {
	return t.Hour()*3600 + t.Minute()*60 + t.Second()
}

// calendarStartOfDay returns midnight of t's calendar day in t's location.
func calendarStartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// calendarStartOfNextDay returns midnight of the calendar day following t, in
// t's location.
func calendarStartOfNextDay(t time.Time) time.Time {
	return calendarStartOfDay(t).AddDate(0, 0, 1)
}

// calendarDate identifies a single calendar day independent of the time of day.
type calendarDate struct {
	year  int
	month time.Month
	day   int
}

// calendarMonthDay identifies a day within a year independent of which year.
type calendarMonthDay struct {
	month time.Month
	day   int
}

// HolidayCalendar excludes whole calendar days. Exclusions have day
// granularity: any instant that falls on an excluded date is excluded
// regardless of its time of day. Dates are compared in the calendar's
// evaluation location.
type HolidayCalendar struct {
	BaseCalendar
	excluded map[calendarDate]struct{}
}

// NewHolidayCalendar returns a HolidayCalendar with no excluded dates.
func NewHolidayCalendar() *HolidayCalendar {
	return &HolidayCalendar{excluded: make(map[calendarDate]struct{})}
}

// AddExcludedDate excludes the calendar day on which t falls. The time of day of
// t is ignored.
func (c *HolidayCalendar) AddExcludedDate(t time.Time) {
	c.excluded[c.calendarDateOf(t)] = struct{}{}
}

// RemoveExcludedDate stops excluding the calendar day on which t falls. It is a
// no-op when that day was not excluded.
func (c *HolidayCalendar) RemoveExcludedDate(t time.Time) {
	delete(c.excluded, c.calendarDateOf(t))
}

// calendarDateOf returns the calendar day of t in the calendar's evaluation
// location.
func (c *HolidayCalendar) calendarDateOf(t time.Time) calendarDate {
	tt := c.calendarEval(t)
	y, m, d := tt.Date()
	return calendarDate{year: y, month: m, day: d}
}

// calendarSelfIncluded reports whether t's day is not among the excluded dates.
func (c *HolidayCalendar) calendarSelfIncluded(t time.Time) bool {
	_, excluded := c.excluded[c.calendarDateOf(t)]
	return !excluded
}

// calendarAdvance returns midnight of the day following t, used to skip an
// excluded day in a single step.
func (c *HolidayCalendar) calendarAdvance(t time.Time) time.Time {
	return calendarStartOfNextDay(c.calendarEval(t))
}

// IsTimeIncluded reports whether t is included, that is whether its day is not
// excluded and the base Calendar also includes it.
func (c *HolidayCalendar) IsTimeIncluded(t time.Time) bool {
	return c.calendarSelfIncluded(t) && c.calendarBaseIncluded(t)
}

// NextIncludedTime returns the earliest instant at or after t that is not on an
// excluded day and is allowed by the base Calendar.
func (c *HolidayCalendar) NextIncludedTime(t time.Time) time.Time {
	return calendarNextIncluded(&c.BaseCalendar, t, c.calendarSelfIncluded, c.calendarAdvance)
}

// DailyCalendar excludes a wall-clock time-of-day range that repeats every day.
// By default the range is the excluded window; Invert flips the sense so that
// only instants inside the range are included. The range may wrap past midnight
// (when its start is later than its end). The range is evaluated in the
// calendar's location.
type DailyCalendar struct {
	BaseCalendar
	startSec int
	endSec   int
	inverted bool
}

// NewDailyCalendar returns a DailyCalendar that excludes the inclusive
// wall-clock range [start, end] every day. When start is later than end the
// range wraps across midnight. It reuses the package TimeOfDay type; the range
// is defined by each value's hour, minute and second.
func NewDailyCalendar(start, end TimeOfDay) *DailyCalendar {
	return &DailyCalendar{
		startSec: calendarTimeOfDaySeconds(start),
		endSec:   calendarTimeOfDaySeconds(end),
	}
}

// calendarTimeOfDaySeconds returns the number of seconds since midnight
// represented by a TimeOfDay.
func calendarTimeOfDaySeconds(t TimeOfDay) int {
	return t.Hour*3600 + t.Minute*60 + t.Second
}

// Invert flips whether the configured range is the excluded window or the only
// included window, and returns the calendar so the call can be chained.
func (c *DailyCalendar) Invert() *DailyCalendar {
	c.inverted = !c.inverted
	return c
}

// calendarInRange reports whether a seconds-since-midnight value falls inside
// the configured range, honoring a range that wraps across midnight.
func (c *DailyCalendar) calendarInRange(sec int) bool {
	if c.startSec <= c.endSec {
		return sec >= c.startSec && sec <= c.endSec
	}
	return sec >= c.startSec || sec <= c.endSec
}

// calendarSelfIncluded reports whether t's time of day is allowed by this
// calendar's own rule. Normally instants inside the range are excluded; when
// inverted only instants inside the range are included.
func (c *DailyCalendar) calendarSelfIncluded(t time.Time) bool {
	in := c.calendarInRange(calendarSecondOfDay(c.calendarEval(t)))
	if c.inverted {
		return in
	}
	return !in
}

// calendarAdvance returns the next instant that could be included, given that t
// is excluded by this calendar's own rule. It jumps to the boundary of the
// excluded window rather than stepping second by second.
func (c *DailyCalendar) calendarAdvance(t time.Time) time.Time {
	tt := c.calendarEval(t)
	day := calendarStartOfDay(tt)
	sec := calendarSecondOfDay(tt)
	if !c.inverted {
		// t is inside the excluded range; jump to the second after it ends.
		if c.startSec <= c.endSec {
			return day.Add(time.Duration(c.endSec+1) * time.Second)
		}
		if sec <= c.endSec {
			return day.Add(time.Duration(c.endSec+1) * time.Second)
		}
		return day.AddDate(0, 0, 1).Add(time.Duration(c.endSec+1) * time.Second)
	}
	// Inverted: only the range is included and t is outside it.
	if c.startSec <= c.endSec {
		if sec < c.startSec {
			return day.Add(time.Duration(c.startSec) * time.Second)
		}
		return day.AddDate(0, 0, 1).Add(time.Duration(c.startSec) * time.Second)
	}
	return day.Add(time.Duration(c.startSec) * time.Second)
}

// IsTimeIncluded reports whether t is included by the time-of-day rule and by
// the base Calendar.
func (c *DailyCalendar) IsTimeIncluded(t time.Time) bool {
	return c.calendarSelfIncluded(t) && c.calendarBaseIncluded(t)
}

// NextIncludedTime returns the earliest instant at or after t whose time of day
// is allowed and that the base Calendar also allows.
func (c *DailyCalendar) NextIncludedTime(t time.Time) time.Time {
	return calendarNextIncluded(&c.BaseCalendar, t, c.calendarSelfIncluded, c.calendarAdvance)
}

// WeeklyCalendar excludes whole days of the week. By default Sunday and Saturday
// are excluded. Exclusions have day granularity and the weekday is determined in
// the calendar's location.
type WeeklyCalendar struct {
	BaseCalendar
	excluded [7]bool
}

// NewWeeklyCalendar returns a WeeklyCalendar with Sunday and Saturday excluded.
func NewWeeklyCalendar() *WeeklyCalendar {
	c := &WeeklyCalendar{}
	c.excluded[time.Sunday] = true
	c.excluded[time.Saturday] = true
	return c
}

// SetDayExcluded sets whether the given weekday is excluded.
func (c *WeeklyCalendar) SetDayExcluded(day time.Weekday, excluded bool) {
	c.excluded[day] = excluded
}

// calendarSelfIncluded reports whether t's weekday is not excluded.
func (c *WeeklyCalendar) calendarSelfIncluded(t time.Time) bool {
	return !c.excluded[c.calendarEval(t).Weekday()]
}

// calendarAdvance returns midnight of the day following t, used to skip an
// excluded weekday in a single step.
func (c *WeeklyCalendar) calendarAdvance(t time.Time) time.Time {
	return calendarStartOfNextDay(c.calendarEval(t))
}

// IsTimeIncluded reports whether t's weekday is not excluded and the base
// Calendar also includes t.
func (c *WeeklyCalendar) IsTimeIncluded(t time.Time) bool {
	return c.calendarSelfIncluded(t) && c.calendarBaseIncluded(t)
}

// NextIncludedTime returns the earliest instant at or after t that falls on a
// non-excluded weekday and is allowed by the base Calendar.
func (c *WeeklyCalendar) NextIncludedTime(t time.Time) time.Time {
	return calendarNextIncluded(&c.BaseCalendar, t, c.calendarSelfIncluded, c.calendarAdvance)
}

// MonthlyCalendar excludes numbered days of the month (1 through 31). Exclusions
// have day granularity and the day of month is determined in the calendar's
// location. Excluding a day that a given month does not have simply never
// matches in that month.
type MonthlyCalendar struct {
	BaseCalendar
	excluded [32]bool
}

// NewMonthlyCalendar returns a MonthlyCalendar with no days excluded.
func NewMonthlyCalendar() *MonthlyCalendar {
	return &MonthlyCalendar{}
}

// SetDayExcluded sets whether the given day of month is excluded. Days outside
// the range 1 through 31 are ignored.
func (c *MonthlyCalendar) SetDayExcluded(day int, excluded bool) {
	if day >= 1 && day <= 31 {
		c.excluded[day] = excluded
	}
}

// calendarSelfIncluded reports whether t's day of month is not excluded.
func (c *MonthlyCalendar) calendarSelfIncluded(t time.Time) bool {
	return !c.excluded[c.calendarEval(t).Day()]
}

// calendarAdvance returns midnight of the day following t, used to skip an
// excluded day of month in a single step.
func (c *MonthlyCalendar) calendarAdvance(t time.Time) time.Time {
	return calendarStartOfNextDay(c.calendarEval(t))
}

// IsTimeIncluded reports whether t's day of month is not excluded and the base
// Calendar also includes t.
func (c *MonthlyCalendar) IsTimeIncluded(t time.Time) bool {
	return c.calendarSelfIncluded(t) && c.calendarBaseIncluded(t)
}

// NextIncludedTime returns the earliest instant at or after t that falls on a
// non-excluded day of month and is allowed by the base Calendar.
func (c *MonthlyCalendar) NextIncludedTime(t time.Time) time.Time {
	return calendarNextIncluded(&c.BaseCalendar, t, c.calendarSelfIncluded, c.calendarAdvance)
}

// AnnualCalendar excludes a specific month and day that recurs every year (for
// example December 25). The year is ignored when matching. Exclusions have day
// granularity and the date is determined in the calendar's location.
type AnnualCalendar struct {
	BaseCalendar
	excluded map[calendarMonthDay]bool
}

// NewAnnualCalendar returns an AnnualCalendar with no days excluded.
func NewAnnualCalendar() *AnnualCalendar {
	return &AnnualCalendar{excluded: make(map[calendarMonthDay]bool)}
}

// SetDayExcluded sets whether the given month and day are excluded every year.
func (c *AnnualCalendar) SetDayExcluded(month time.Month, day int, excluded bool) {
	key := calendarMonthDay{month: month, day: day}
	if excluded {
		c.excluded[key] = true
	} else {
		delete(c.excluded, key)
	}
}

// calendarSelfIncluded reports whether t's month and day are not excluded.
func (c *AnnualCalendar) calendarSelfIncluded(t time.Time) bool {
	tt := c.calendarEval(t)
	return !c.excluded[calendarMonthDay{month: tt.Month(), day: tt.Day()}]
}

// calendarAdvance returns midnight of the day following t, used to skip an
// excluded annual date in a single step.
func (c *AnnualCalendar) calendarAdvance(t time.Time) time.Time {
	return calendarStartOfNextDay(c.calendarEval(t))
}

// IsTimeIncluded reports whether t's month and day are not excluded and the base
// Calendar also includes t.
func (c *AnnualCalendar) IsTimeIncluded(t time.Time) bool {
	return c.calendarSelfIncluded(t) && c.calendarBaseIncluded(t)
}

// NextIncludedTime returns the earliest instant at or after t that does not fall
// on an excluded annual date and is allowed by the base Calendar.
func (c *AnnualCalendar) NextIncludedTime(t time.Time) time.Time {
	return calendarNextIncluded(&c.BaseCalendar, t, c.calendarSelfIncluded, c.calendarAdvance)
}

// CronCalendar excludes every instant that matches a cron expression. It reuses
// the package cron parser, so any expression accepted by ParseCron is accepted
// here. Matching is performed at one-second granularity in the calendar's
// location.
type CronCalendar struct {
	BaseCalendar
	expr *CronExpression
}

// NewCronCalendar returns a CronCalendar that excludes instants matching expr.
// It returns the error from ParseCron when expr is invalid.
func NewCronCalendar(expr string) (*CronCalendar, error) {
	ce, err := ParseCron(expr)
	if err != nil {
		return nil, err
	}
	return &CronCalendar{expr: ce}, nil
}

// calendarMatches reports whether the whole second of t matches the expression.
// It uses CronExpression.Next on the instant one second earlier: that call
// returns the first match strictly after it, which equals t exactly when t
// itself matches.
func (c *CronCalendar) calendarMatches(t time.Time) bool {
	sec := c.calendarEval(t).Truncate(time.Second)
	n := c.expr.Next(sec.Add(-time.Second))
	return !n.IsZero() && n.Equal(sec)
}

// calendarSelfIncluded reports whether t does not match the expression.
func (c *CronCalendar) calendarSelfIncluded(t time.Time) bool {
	return !c.calendarMatches(t)
}

// calendarAdvance returns the whole second following t, used to step past a
// matched (excluded) instant.
func (c *CronCalendar) calendarAdvance(t time.Time) time.Time {
	return c.calendarEval(t).Truncate(time.Second).Add(time.Second)
}

// IsTimeIncluded reports whether t does not match the expression and the base
// Calendar also includes t.
func (c *CronCalendar) IsTimeIncluded(t time.Time) bool {
	return c.calendarSelfIncluded(t) && c.calendarBaseIncluded(t)
}

// NextIncludedTime returns the earliest instant at or after t that does not
// match the expression and is allowed by the base Calendar.
func (c *CronCalendar) NextIncludedTime(t time.Time) time.Time {
	return calendarNextIncluded(&c.BaseCalendar, t, c.calendarSelfIncluded, c.calendarAdvance)
}

// CalendarTrigger decorates another Trigger with a Calendar so that scheduled
// fire times never land on an instant the Calendar excludes. It delegates every
// Trigger method to the wrapped trigger but, after the wrapped trigger produces
// a next fire time, advances the wrapped trigger past any excluded instant using
// the Calendar. The wrapped trigger and any scheduler using it are left
// untouched; the exclusion behavior lives entirely in this decorator.
type CalendarTrigger struct {
	inner    Trigger
	calendar Calendar
}

// WithCalendar returns a CalendarTrigger that fires like inner but skips every
// instant excluded by cal. A nil cal disables filtering and makes the decorator
// a transparent pass-through.
func WithCalendar(inner Trigger, cal Calendar) *CalendarTrigger {
	return &CalendarTrigger{inner: inner, calendar: cal}
}

// calendarSkipExcluded advances the inner trigger, starting from its already
// computed next fire time, until that fire time is included by the Calendar,
// the inner trigger is exhausted, or it stops making forward progress. It uses
// Calendar.NextIncludedTime to find the target included instant and steps the
// inner trigger (via Triggered) up to it, so an excluded region is crossed
// without testing every intermediate fire time individually.
func (t *CalendarTrigger) calendarSkipExcluded(next time.Time) time.Time {
	if t.calendar == nil {
		return next
	}
	for guard := 0; guard < calendarMaxTriggerAdvances; guard++ {
		if next.IsZero() || t.calendar.IsTimeIncluded(next) {
			return next
		}
		target := t.calendar.NextIncludedTime(next)
		for inner := 0; inner < calendarMaxTriggerAdvances; inner++ {
			advanced := t.inner.Triggered(next)
			if advanced.IsZero() {
				return advanced
			}
			if !advanced.After(next) {
				// The inner trigger is not moving forward; stop rather than
				// loop forever.
				return advanced
			}
			next = advanced
			if !next.Before(target) {
				break
			}
		}
	}
	return next
}

// Key implements Trigger by delegating to the wrapped trigger.
func (t *CalendarTrigger) Key() Key { return t.inner.Key() }

// JobKey implements Trigger by delegating to the wrapped trigger.
func (t *CalendarTrigger) JobKey() Key { return t.inner.JobKey() }

// Description implements Trigger by delegating to the wrapped trigger.
func (t *CalendarTrigger) Description() string { return t.inner.Description() }

// MisfirePolicy implements Trigger by delegating to the wrapped trigger.
func (t *CalendarTrigger) MisfirePolicy() MisfirePolicy { return t.inner.MisfirePolicy() }

// NextFireTime implements Trigger. It returns the wrapped trigger's next fire
// time, which is always an included instant (or the zero time) because every
// mutating call re-applies the Calendar filter.
func (t *CalendarTrigger) NextFireTime() time.Time { return t.inner.NextFireTime() }

// PreviousFireTime implements Trigger by delegating to the wrapped trigger.
func (t *CalendarTrigger) PreviousFireTime() time.Time { return t.inner.PreviousFireTime() }

// WillFireAgain implements Trigger by delegating to the wrapped trigger.
func (t *CalendarTrigger) WillFireAgain() bool { return t.inner.WillFireAgain() }

// ComputeFirstFireTime implements Trigger. It computes the wrapped trigger's
// first fire time and then advances past any excluded instant so the first fire
// is never on an excluded time.
func (t *CalendarTrigger) ComputeFirstFireTime(now time.Time) time.Time {
	return t.calendarSkipExcluded(t.inner.ComputeFirstFireTime(now))
}

// Triggered implements Trigger. It advances the wrapped trigger and then skips
// forward over any excluded instant so the returned next fire time is included.
func (t *CalendarTrigger) Triggered(now time.Time) time.Time {
	return t.calendarSkipExcluded(t.inner.Triggered(now))
}

// UpdateAfterMisfire implements Trigger. It applies the wrapped trigger's
// misfire handling and then re-applies the Calendar filter so the rescheduled
// fire time is never on an excluded time.
func (t *CalendarTrigger) UpdateAfterMisfire(now time.Time) time.Time {
	return t.calendarSkipExcluded(t.inner.UpdateAfterMisfire(now))
}
