package quartz

import (
	"testing"
	"time"
)

// Compile-time assertion that DailyTimeIntervalTrigger satisfies the full
// Trigger interface.
var _ Trigger = (*DailyTimeIntervalTrigger)(nil)

// daily_time_interval_triggerCollect drives the trigger from its first fire and
// returns up to max fire times.
func daily_time_interval_triggerCollect(trig *DailyTimeIntervalTrigger, now time.Time, max int) []time.Time {
	var fires []time.Time
	trig.ComputeFirstFireTime(now)
	for trig.WillFireAgain() && len(fires) < max {
		fires = append(fires, trig.NextFireTime())
		trig.Triggered(trig.NextFireTime())
	}
	return fires
}

// daily_time_interval_triggerEqualTimes reports the first index at which two
// fire-time slices differ, or -1 when they are equal.
func daily_time_interval_triggerEqualTimes(got, want []time.Time) int {
	if len(got) != len(want) {
		return -1
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			return i
		}
	}
	return -2
}

func TestDailyTimeIntervalTriggerWindows(t *testing.T) {
	// 2026-01-01 is a Thursday; all cases use all seven weekdays.
	now := mustTime(t, "2026-01-01 00:00:00")

	cases := []struct {
		name  string
		start TimeOfDay
		end   TimeOfDay
		unit  IntervalUnit
		count int
		want  []time.Time
	}{
		{
			name:  "hourly_window_09_11_rolls_to_next_day",
			start: NewTimeOfDay(9, 0, 0),
			end:   NewTimeOfDay(11, 0, 0),
			unit:  IntervalHour,
			count: 1,
			want: []time.Time{
				mustTime(t, "2026-01-01 09:00:00"),
				mustTime(t, "2026-01-01 10:00:00"),
				mustTime(t, "2026-01-01 11:00:00"),
				mustTime(t, "2026-01-02 09:00:00"),
				mustTime(t, "2026-01-02 10:00:00"),
				mustTime(t, "2026-01-02 11:00:00"),
				mustTime(t, "2026-01-03 09:00:00"),
			},
		},
		{
			name:  "minute_step_15_window_09_00_09_30",
			start: NewTimeOfDay(9, 0, 0),
			end:   NewTimeOfDay(9, 30, 0),
			unit:  IntervalMinute,
			count: 15,
			want: []time.Time{
				mustTime(t, "2026-01-01 09:00:00"),
				mustTime(t, "2026-01-01 09:15:00"),
				mustTime(t, "2026-01-01 09:30:00"),
				mustTime(t, "2026-01-02 09:00:00"),
				mustTime(t, "2026-01-02 09:15:00"),
			},
		},
		{
			name:  "second_step_30_window_one_minute",
			start: NewTimeOfDay(0, 0, 0),
			end:   NewTimeOfDay(0, 1, 0),
			unit:  IntervalSecond,
			count: 30,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2026-01-01 00:00:30"),
				mustTime(t, "2026-01-01 00:01:00"),
				mustTime(t, "2026-01-02 00:00:00"),
			},
		},
		{
			name:  "degenerate_window_end_equals_start_fires_once_per_day",
			start: NewTimeOfDay(8, 0, 0),
			end:   NewTimeOfDay(8, 0, 0),
			unit:  IntervalHour,
			count: 1,
			want: []time.Time{
				mustTime(t, "2026-01-01 08:00:00"),
				mustTime(t, "2026-01-02 08:00:00"),
				mustTime(t, "2026-01-03 08:00:00"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"), tc.start, tc.end, tc.unit, tc.count)
			got := daily_time_interval_triggerCollect(trig, now, len(tc.want))
			if idx := daily_time_interval_triggerEqualTimes(got, tc.want); idx != -2 {
				t.Fatalf("fires mismatch (idx %d): got %v, want %v", idx, got, tc.want)
			}
			if trig.TimesTriggered() != len(tc.want) {
				t.Errorf("TimesTriggered = %d, want %d", trig.TimesTriggered(), len(tc.want))
			}
		})
	}
}

func TestDailyTimeIntervalTriggerDaysOfWeek(t *testing.T) {
	now := mustTime(t, "2026-01-01 00:00:00") // Thursday
	trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(8, 0, 0), NewTimeOfDay(8, 0, 0), IntervalHour, 1).
		OnDaysOfWeek(time.Monday, time.Wednesday, time.Friday)

	got := daily_time_interval_triggerCollect(trig, now, 4)
	want := []time.Time{
		mustTime(t, "2026-01-02 08:00:00"), // Friday
		mustTime(t, "2026-01-05 08:00:00"), // Monday
		mustTime(t, "2026-01-07 08:00:00"), // Wednesday
		mustTime(t, "2026-01-09 08:00:00"), // Friday
	}
	if idx := daily_time_interval_triggerEqualTimes(got, want); idx != -2 {
		t.Fatalf("fires mismatch (idx %d): got %v, want %v", idx, got, want)
	}
}

func TestDailyTimeIntervalTriggerRepeatCount(t *testing.T) {
	now := mustTime(t, "2026-01-01 00:00:00")
	// A repeat count of 2 yields 3 total fires (initial plus two repeats).
	trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(9, 0, 0), NewTimeOfDay(11, 0, 0), IntervalHour, 1).
		WithRepeatCount(2)

	got := daily_time_interval_triggerCollect(trig, now, 10)
	want := []time.Time{
		mustTime(t, "2026-01-01 09:00:00"),
		mustTime(t, "2026-01-01 10:00:00"),
		mustTime(t, "2026-01-01 11:00:00"),
	}
	if idx := daily_time_interval_triggerEqualTimes(got, want); idx != -2 {
		t.Fatalf("fires mismatch (idx %d): got %v, want %v", idx, got, want)
	}
	if trig.WillFireAgain() {
		t.Errorf("expected exhausted trigger, next = %v", trig.NextFireTime())
	}
	if !trig.PreviousFireTime().Equal(mustTime(t, "2026-01-01 11:00:00")) {
		t.Errorf("prev = %v, want 2026-01-01 11:00:00", trig.PreviousFireTime())
	}
}

func TestDailyTimeIntervalTriggerEndTime(t *testing.T) {
	now := mustTime(t, "2026-01-01 00:00:00")
	trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(9, 0, 0), NewTimeOfDay(11, 0, 0), IntervalHour, 1).
		EndingAt(mustTime(t, "2026-01-01 10:30:00"))

	got := daily_time_interval_triggerCollect(trig, now, 10)
	want := []time.Time{
		mustTime(t, "2026-01-01 09:00:00"),
		mustTime(t, "2026-01-01 10:00:00"),
	}
	if idx := daily_time_interval_triggerEqualTimes(got, want); idx != -2 {
		t.Fatalf("fires mismatch (idx %d): got %v, want %v", idx, got, want)
	}
	if trig.WillFireAgain() {
		t.Errorf("expected exhausted trigger, next = %v", trig.NextFireTime())
	}
}

func TestDailyTimeIntervalTriggerComputeFirstFireTime(t *testing.T) {
	now := mustTime(t, "2026-01-01 00:00:00")

	t.Run("starts_at_max_of_now_and_start_time", func(t *testing.T) {
		// A configured start time of 09:30 is later than now; the first slot at
		// or after 09:30 is the 10:00 step (09:30 is not on the step grid).
		trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
			NewTimeOfDay(9, 0, 0), NewTimeOfDay(11, 0, 0), IntervalHour, 1).
			StartingAt(mustTime(t, "2026-01-01 09:30:00"))
		got := daily_time_interval_triggerCollect(trig, now, 3)
		want := []time.Time{
			mustTime(t, "2026-01-01 10:00:00"),
			mustTime(t, "2026-01-01 11:00:00"),
			mustTime(t, "2026-01-02 09:00:00"),
		}
		if idx := daily_time_interval_triggerEqualTimes(got, want); idx != -2 {
			t.Fatalf("fires mismatch (idx %d): got %v, want %v", idx, got, want)
		}
	})

	t.Run("no_active_days_never_fires", func(t *testing.T) {
		trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
			NewTimeOfDay(9, 0, 0), NewTimeOfDay(11, 0, 0), IntervalHour, 1)
		// Force an empty active-day set by replacing the slice directly; the
		// public OnDaysOfWeek keeps the default when given no arguments.
		trig.daysOfWeek = nil
		if first := trig.ComputeFirstFireTime(now); !first.IsZero() {
			t.Fatalf("first = %v, want zero", first)
		}
		if trig.WillFireAgain() {
			t.Fatalf("expected no fire, next = %v", trig.NextFireTime())
		}
	})
}

func TestDailyTimeIntervalTriggerMisfire(t *testing.T) {
	now := mustTime(t, "2026-01-01 00:00:00")
	newTrig := func(p MisfirePolicy) *DailyTimeIntervalTrigger {
		return NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
			NewTimeOfDay(9, 0, 0), NewTimeOfDay(11, 0, 0), IntervalHour, 1).
			WithMisfirePolicy(p)
	}

	t.Run("ignore_leaves_next_unchanged", func(t *testing.T) {
		trig := newTrig(MisfireIgnore)
		trig.ComputeFirstFireTime(now)
		want := mustTime(t, "2026-01-01 09:00:00")
		if got := trig.UpdateAfterMisfire(mustTime(t, "2026-06-01 12:00:00")); !got.Equal(want) {
			t.Fatalf("misfire next = %v, want %v (unchanged)", got, want)
		}
	})

	t.Run("fire_now_sets_now", func(t *testing.T) {
		trig := newTrig(MisfireFireNow)
		trig.ComputeFirstFireTime(now)
		nowLate := mustTime(t, "2026-06-01 12:00:00")
		if got := trig.UpdateAfterMisfire(nowLate); !got.Equal(nowLate) {
			t.Fatalf("misfire next = %v, want %v (now)", got, nowLate)
		}
	})

	t.Run("do_nothing_advances_past_now", func(t *testing.T) {
		trig := newTrig(MisfireDoNothing)
		trig.ComputeFirstFireTime(now)
		// now is Jan 2 09:30; the next in-window slot strictly after is 10:00.
		got := trig.UpdateAfterMisfire(mustTime(t, "2026-01-02 09:30:00"))
		want := mustTime(t, "2026-01-02 10:00:00")
		if !got.Equal(want) {
			t.Fatalf("misfire next = %v, want %v", got, want)
		}
	})
}

func TestDailyTimeIntervalTriggerBuildersAndGetters(t *testing.T) {
	now := mustTime(t, "2026-01-01 00:00:00")
	loc := time.FixedZone("X", 2*3600)
	start := mustTime(t, "2026-01-01 00:00:00")
	end := mustTime(t, "2026-12-31 00:00:00")
	trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(9, 0, 0), NewTimeOfDay(17, 30, 15), IntervalMinute, 30).
		In(loc).
		StartingAt(start).
		EndingAt(end).
		OnDaysOfWeek(time.Tuesday, time.Thursday).
		WithRepeatCount(5).
		WithDescription("desc").
		WithMisfirePolicy(MisfireDoNothing)

	if trig.Key() != NewKey("t") || trig.JobKey() != NewKey("j") {
		t.Errorf("Key/JobKey = %v/%v", trig.Key(), trig.JobKey())
	}
	if trig.Description() != "desc" {
		t.Errorf("Description = %q", trig.Description())
	}
	if trig.MisfirePolicy() != MisfireDoNothing {
		t.Errorf("MisfirePolicy = %v", trig.MisfirePolicy())
	}
	if trig.Location() != loc {
		t.Errorf("Location = %v, want %v", trig.Location(), loc)
	}
	if trig.Unit() != IntervalMinute || trig.Count() != 30 {
		t.Errorf("Unit/Count = %v/%d", trig.Unit(), trig.Count())
	}
	if trig.RepeatCount() != 5 {
		t.Errorf("RepeatCount = %d, want 5", trig.RepeatCount())
	}
	if !trig.StartTime().Equal(start) || !trig.EndTime().Equal(end) {
		t.Errorf("StartTime/EndTime = %v/%v", trig.StartTime(), trig.EndTime())
	}
	if sod := trig.StartTimeOfDay(); sod != NewTimeOfDay(9, 0, 0) {
		t.Errorf("StartTimeOfDay = %+v", sod)
	}
	if eod := trig.EndTimeOfDay(); eod != NewTimeOfDay(17, 30, 15) {
		t.Errorf("EndTimeOfDay = %+v", eod)
	}
	if days := trig.DaysOfWeek(); len(days) != 2 || days[0] != time.Tuesday || days[1] != time.Thursday {
		t.Errorf("DaysOfWeek = %v", days)
	}

	// The returned DaysOfWeek slice is a copy: mutating it must not change state.
	trig.DaysOfWeek()[0] = time.Sunday
	if trig.DaysOfWeek()[0] != time.Tuesday {
		t.Errorf("DaysOfWeek leaked internal slice")
	}

	// A nil location is ignored, leaving the previously set location in place.
	if trig.In(nil).Location() != loc {
		t.Errorf("In(nil) changed location to %v", trig.Location())
	}

	// OnDaysOfWeek with no arguments leaves the current set unchanged.
	if days := trig.OnDaysOfWeek().DaysOfWeek(); len(days) != 2 {
		t.Errorf("OnDaysOfWeek() changed the active-day set to %v", days)
	}

	_ = now
}

func TestDailyTimeIntervalTriggerWallClockInLocation(t *testing.T) {
	// In a fixed +02:00 zone, a 09:00 wall-clock window start maps to 07:00 UTC.
	loc := time.FixedZone("+02", 2*3600)
	now := mustTime(t, "2026-01-01 00:00:00")
	trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(9, 0, 0), NewTimeOfDay(9, 0, 0), IntervalHour, 1).
		In(loc)

	got := daily_time_interval_triggerCollect(trig, now, 2)
	want := []time.Time{
		mustTime(t, "2026-01-01 07:00:00"),
		mustTime(t, "2026-01-02 07:00:00"),
	}
	if idx := daily_time_interval_triggerEqualTimes(got, want); idx != -2 {
		t.Fatalf("fires mismatch (idx %d): got %v, want %v", idx, got, want)
	}
	// The wall-clock hour in the location is 09:00.
	if h := got[0].In(loc).Hour(); h != 9 {
		t.Errorf("local hour = %d, want 9", h)
	}
}

func TestDailyTimeIntervalTriggerDaylightSaving(t *testing.T) {
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("time zone data unavailable: %v", err)
	}
	// A single daily 12:00 local fire across the 2026-03-08 spring-forward
	// transition (EST -> EDT) must keep the local hour fixed at 12:00.
	start := time.Date(2026, time.March, 7, 0, 0, 0, 0, nyc)
	trig := NewDailyTimeIntervalTrigger(NewKey("t"), NewKey("j"),
		NewTimeOfDay(12, 0, 0), NewTimeOfDay(12, 0, 0), IntervalHour, 1).
		In(nyc)
	trig.ComputeFirstFireTime(start)

	first := trig.NextFireTime().In(nyc)
	if first.Day() != 7 || first.Hour() != 12 {
		t.Fatalf("first = %v, want 2026-03-07 12:00 local", first)
	}
	trig.Triggered(trig.NextFireTime())
	second := trig.NextFireTime().In(nyc)
	if second.Day() != 8 || second.Hour() != 12 {
		t.Fatalf("second = %v, want 2026-03-08 12:00 local", second)
	}
	// 12:00 EST is 17:00 UTC; 12:00 EDT is 16:00 UTC. The absolute UTC instant
	// shifts by the one-hour offset change while the wall-clock hour is fixed.
	wantUTC := time.Date(2026, time.March, 8, 16, 0, 0, 0, time.UTC)
	if !trig.NextFireTime().Equal(wantUTC) {
		t.Fatalf("second UTC = %v, want %v", trig.NextFireTime().UTC(), wantUTC)
	}
}
