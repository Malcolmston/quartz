package quartz

import (
	"testing"
	"time"
)

// This file encodes known-answer vectors taken directly from the upstream
// project this package mirrors: the Java Quartz scheduler
// (github.com/quartz-scheduler/quartz, branch quartz-2.3.x). Each TestParity...
// function translates concrete assertions from an upstream JUnit test into a
// deterministic Go assertion against this package's real exported API. The
// upstream source file is named in each test's comment.
//
// Times are constructed in fixed locations (mostly UTC) so the vectors are
// deterministic regardless of the host time zone. Where an upstream vector
// exercises a feature this Unix-cron-style port deliberately does not implement
// (the Quartz L / W / # tokens, the trailing year field, or Java's 1-7
// day-of-week numbering), the divergence is documented in
// TestParityCronUnsupportedQuartzTokens rather than silently ignored.

// parityFireTimes drives a trigger through the Trigger interface exactly as the
// upstream TriggerUtils.computeFireTimes helper does: it computes the first fire
// time from the given reference clock, then repeatedly records the current fire
// time and advances. It returns up to n fire times (fewer if the trigger is
// exhausted first).
func parityFireTimes(tr Trigger, now time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	ft := tr.ComputeFirstFireTime(now)
	for i := 0; i < n && !ft.IsZero(); i++ {
		out = append(out, ft)
		ft = tr.Triggered(ft)
	}
	return out
}

// TestParityCronIsSatisfiedBy mirrors CronExpressionTest.testIsSatisfiedBy. The
// upstream expression is "0 15 10 * * ? 2005"; this port has no trailing year
// field, so the year component is dropped and the surviving second/minute/hour
// assertions are checked. June 1 2005 at 10:15:00 satisfies the schedule while
// 10:16:00 and 10:14:00 do not.
func TestParityCronIsSatisfiedBy(t *testing.T) {
	ce := MustParseCron("0 15 10 * * ?")
	base := time.Date(2005, time.June, 1, 10, 15, 0, 0, time.UTC)
	if !ce.IsSatisfiedBy(base) {
		t.Errorf("IsSatisfiedBy(%v) = false, want true", base)
	}
	if ce.IsSatisfiedBy(base.Add(time.Minute)) {
		t.Errorf("IsSatisfiedBy(10:16) = true, want false")
	}
	if ce.IsSatisfiedBy(base.Add(-time.Minute)) {
		t.Errorf("IsSatisfiedBy(10:14) = true, want false")
	}
}

// TestParityCronNextSequence pins the concrete next-fire progression implied by
// CronExpressionTest.testIsSatisfiedBy: strictly before 10:15 the next fire is
// 10:15:00, and strictly at 10:15:00 the following fire rolls to the next day.
func TestParityCronNextSequence(t *testing.T) {
	ce := MustParseCron("0 15 10 * * ?")
	before := time.Date(2005, time.June, 1, 10, 14, 59, 0, time.UTC)
	want := time.Date(2005, time.June, 1, 10, 15, 0, 0, time.UTC)
	if got := ce.Next(before); !got.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", before, got, want)
	}
	wantNext := time.Date(2005, time.June, 2, 10, 15, 0, 0, time.UTC)
	if got := ce.Next(want); !got.Equal(wantNext) {
		t.Errorf("Next(%v) = %v, want %v", want, got, wantNext)
	}
}

// TestParityCronInvalidMonth mirrors CronExpressionTest.testQuartz574, which
// asserts that a non-existent month name is rejected at parse time.
func TestParityCronInvalidMonth(t *testing.T) {
	if _, err := ParseCron("* * * * Foo ?"); err == nil {
		t.Errorf("ParseCron with month=Foo succeeded, want error")
	}
	if _, err := ParseCron("* * * * Jan-Foo ?"); err == nil {
		t.Errorf("ParseCron with month=Jan-Foo succeeded, want error")
	}
}

// TestParityCronSlashRequiresInteger mirrors the "'/' must be followed by an
// integer." cases from CronExpressionTest.testSecRangeIntervalAfterSlash and
// its sibling per-field tests: a step token with no integer is rejected.
func TestParityCronSlashRequiresInteger(t *testing.T) {
	for _, expr := range []string{
		"0/ 0 8 * * ?",
		"0 0/ 8 * * ?",
		"0 0 0/ * * ?",
	} {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) succeeded, want error", expr)
		}
	}
}

// TestParitySimpleTriggerFireTimeAfter mirrors
// SimpleTriggerTest.testGetFireTimeAfter: start at the epoch, a 10-unit
// interval and a repeat count of 4 (five fires at 0,10,20,30,40). The first
// fire strictly after the 34-unit mark is the 40-unit fire. The upstream units
// are milliseconds; nanoseconds are used here so the arithmetic is identical.
func TestParitySimpleTriggerFireTimeAfter(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	tr := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, 10*time.Nanosecond, 4)
	after := start.Add(34 * time.Nanosecond)
	want := start.Add(40 * time.Nanosecond)
	if got := tr.FireTimeAfter(after); !got.Equal(want) {
		t.Errorf("FireTimeAfter(34) = %v, want %v", got.UnixNano(), want.UnixNano())
	}
	// Strictly-after semantics: querying exactly on a fire returns the next.
	if got := tr.FireTimeAfter(start.Add(30 * time.Nanosecond)); !got.Equal(want) {
		t.Errorf("FireTimeAfter(30) = %v, want %v", got.UnixNano(), want.UnixNano())
	}
	// Past the last fire the trigger is exhausted.
	if got := tr.FireTimeAfter(start.Add(40 * time.Nanosecond)); !got.IsZero() {
		t.Errorf("FireTimeAfter(40) = %v, want zero", got)
	}
}

// calStart is the reference start time shared by the calendar-interval vectors,
// matching the upstream "2005, JUNE, 1, 9, 30, 17" start with milliseconds
// cleared.
var calStart = time.Date(2005, time.June, 1, 9, 30, 17, 0, time.UTC)

// TestParityCalendarIntervalGetFireTimeAfter mirrors the per-unit
// get-fire-time-after tests in CalendarIntervalTriggerTest
// (testDailyIntervalGetFireTimeAfter, testWeeklyIntervalGetFireTimeAfter, ...).
// Each case asserts that a specific index in the computed fire-time list equals
// the start time advanced by (index * interval) of the given unit. In UTC the
// sub-day and day/week advances are plain durations, giving independent
// known answers; month/year advances use calendar arithmetic exactly as the
// upstream Calendar.add does.
func TestParityCalendarIntervalGetFireTimeAfter(t *testing.T) {
	cases := []struct {
		name  string
		unit  IntervalUnit
		count int
		n     int // number of fire times to compute
		index int // index asserted
		want  time.Time
	}{
		{"secondly-100", IntervalSecond, 100, 6, 4, calStart.Add(400 * time.Second)},
		{"minutely-100", IntervalMinute, 100, 6, 4, calStart.Add(400 * time.Minute)},
		{"hourly-100", IntervalHour, 100, 6, 4, calStart.Add(400 * time.Hour)},
		{"daily-90", IntervalDay, 90, 6, 4, calStart.Add(360 * 24 * time.Hour)},
		{"weekly-6", IntervalWeek, 6, 7, 4, calStart.Add(24 * 7 * 24 * time.Hour)},
		{"monthly-5", IntervalMonth, 5, 6, 5, calStart.AddDate(0, 25, 0)},
		{"yearly-2", IntervalYear, 2, 4, 2, calStart.AddDate(4, 0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), calStart, c.unit, c.count)
			fires := parityFireTimes(tr, time.Time{}, c.n)
			if len(fires) <= c.index {
				t.Fatalf("only %d fire times, need index %d", len(fires), c.index)
			}
			if !fires[c.index].Equal(c.want) {
				t.Errorf("fire[%d] = %v, want %v", c.index, fires[c.index], c.want)
			}
			// fire[0] is always the start time.
			if !fires[0].Equal(calStart) {
				t.Errorf("fire[0] = %v, want start %v", fires[0], calStart)
			}
		})
	}
}

// TestParityCalendarIntervalFireTimeAfterBoundary mirrors
// CalendarIntervalTriggerTest.testQTZ331FireTimeAfterBoundary: for a daily
// trigger the fire time after the start time is start+1day, and the fire time
// after a moment just before that (500ms earlier) is still start+1day.
func TestParityCalendarIntervalFireTimeAfterBoundary(t *testing.T) {
	start := time.Date(2013, time.February, 15, 0, 0, 0, 0, time.UTC)
	next := start.AddDate(0, 0, 1)
	tr := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1)

	if got := tr.FireTimeAfter(start); !got.Equal(next) {
		t.Errorf("FireTimeAfter(start) = %v, want %v", got, next)
	}
	after := next.Add(-500 * time.Millisecond)
	if got := tr.FireTimeAfter(after); !got.Equal(next) {
		t.Errorf("FireTimeAfter(next-500ms) = %v, want %v", got, next)
	}
}

// TestParityDailyTimeIntervalNormalExample mirrors
// DailyTimeIntervalTriggerImplTest.testNormalExample: a window of 08:00-11:00
// stepping every 72 minutes fires three times a day (08:00, 09:12, 10:24). The
// first 48 fires therefore span 16 days, ending at 10:24 on Jan 16 2011.
func TestParityDailyTimeIntervalNormalExample(t *testing.T) {
	start := time.Date(2011, time.January, 1, 0, 0, 0, 0, time.UTC)
	tr := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(8, 0, 0), NewTimeOfDay(11, 0, 0), IntervalMinute, 72).
		StartingAt(start)

	fires := parityFireTimes(tr, time.Time{}, 48)
	if len(fires) != 48 {
		t.Fatalf("computed %d fire times, want 48", len(fires))
	}
	want0 := time.Date(2011, time.January, 1, 8, 0, 0, 0, time.UTC)
	if !fires[0].Equal(want0) {
		t.Errorf("fire[0] = %v, want %v", fires[0], want0)
	}
	want47 := time.Date(2011, time.January, 16, 10, 24, 0, 0, time.UTC)
	if !fires[47].Equal(want47) {
		t.Errorf("fire[47] = %v, want %v", fires[47], want47)
	}
}

// TestParityDailyTimeIntervalStartTimeWithoutStartTimeOfDay mirrors
// DailyTimeIntervalTriggerImplTest.testStartTimeWithoutStartTimeOfDay: with no
// explicit window the day runs 00:00-23:59 stepping every 60 minutes, so 48
// fires span two days ending at 23:00 on Jan 2 2011.
func TestParityDailyTimeIntervalStartTimeWithoutStartTimeOfDay(t *testing.T) {
	start := time.Date(2011, time.January, 1, 0, 0, 0, 0, time.UTC)
	tr := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(0, 0, 0), NewTimeOfDay(23, 59, 59), IntervalMinute, 60).
		StartingAt(start)

	fires := parityFireTimes(tr, time.Time{}, 48)
	if len(fires) != 48 {
		t.Fatalf("computed %d fire times, want 48", len(fires))
	}
	want0 := time.Date(2011, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !fires[0].Equal(want0) {
		t.Errorf("fire[0] = %v, want %v", fires[0], want0)
	}
	want47 := time.Date(2011, time.January, 2, 23, 0, 0, 0, time.UTC)
	if !fires[47].Equal(want47) {
		t.Errorf("fire[47] = %v, want %v", fires[47], want47)
	}
}

// TestParityCronUnsupportedQuartzTokens documents, as an honest ceiling, the
// upstream CronExpression features this Unix-cron-style port does not implement:
// the L (last), W (weekday) and # (nth weekday) tokens exercised by
// CronExpressionTest.testLastDayOffset and testQuartz640, and the trailing year
// field. These require a different cron dialect than the deterministic
// bitmask parser used here; supporting them would change the documented field
// grammar rather than fix a bug, so the vectors are recorded and skipped.
func TestParityCronUnsupportedQuartzTokens(t *testing.T) {
	for _, expr := range []string{
		"0 15 10 L-2 * ?",  // last-day offset
		"0 15 10 L-1W * ?", // nearest weekday to a last-day offset
		"0 43 9 ? * 5L",    // last Friday of the month
	} {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) unexpectedly succeeded; the port's grammar changed", expr)
		}
	}
	t.Skip("Quartz L/W/# day tokens and the trailing year field are outside this Unix-cron-style port's documented grammar")
}
