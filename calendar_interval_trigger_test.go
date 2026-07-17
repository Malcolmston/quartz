package quartz

import (
	"testing"
	"time"
)

// Compile-time assertion that CalendarIntervalTrigger satisfies the full
// Trigger interface.
var _ Trigger = (*CalendarIntervalTrigger)(nil)

// collectCalendarIntervalTriggerFires drives the trigger and returns up to max
// fire times, starting from the first fire time.
func collectCalendarIntervalTriggerFires(trig *CalendarIntervalTrigger, start time.Time, max int) []time.Time {
	var fires []time.Time
	trig.ComputeFirstFireTime(start)
	for trig.WillFireAgain() && len(fires) < max {
		fires = append(fires, trig.NextFireTime())
		trig.Triggered(trig.NextFireTime())
	}
	return fires
}

func TestCalendarIntervalTriggerAdvancement(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")

	cases := []struct {
		name  string
		unit  IntervalUnit
		count int
		want  []time.Time
	}{
		{
			name:  "hour_step_6_wraps_day",
			unit:  IntervalHour,
			count: 6,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2026-01-01 06:00:00"),
				mustTime(t, "2026-01-01 12:00:00"),
				mustTime(t, "2026-01-01 18:00:00"),
				mustTime(t, "2026-01-02 00:00:00"),
			},
		},
		{
			name:  "minute_step_90",
			unit:  IntervalMinute,
			count: 90,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2026-01-01 01:30:00"),
				mustTime(t, "2026-01-01 03:00:00"),
			},
		},
		{
			name:  "second_step_45",
			unit:  IntervalSecond,
			count: 45,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2026-01-01 00:00:45"),
				mustTime(t, "2026-01-01 00:01:30"),
			},
		},
		{
			name:  "day_step_1_crosses_month",
			unit:  IntervalDay,
			count: 1,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2026-01-02 00:00:00"),
				mustTime(t, "2026-01-03 00:00:00"),
			},
		},
		{
			name:  "week_step_2_is_14_days",
			unit:  IntervalWeek,
			count: 2,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2026-01-15 00:00:00"),
				mustTime(t, "2026-01-29 00:00:00"),
			},
		},
		{
			name:  "month_step_1_honors_month_length",
			unit:  IntervalMonth,
			count: 1,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2026-02-01 00:00:00"),
				mustTime(t, "2026-03-01 00:00:00"),
			},
		},
		{
			name:  "year_step_1",
			unit:  IntervalYear,
			count: 1,
			want: []time.Time{
				mustTime(t, "2026-01-01 00:00:00"),
				mustTime(t, "2027-01-01 00:00:00"),
				mustTime(t, "2028-01-01 00:00:00"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, tc.unit, tc.count)
			got := collectCalendarIntervalTriggerFires(trig, start, len(tc.want))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d fires, want %d: %v", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if !got[i].Equal(tc.want[i]) {
					t.Errorf("fire %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
			if trig.TimesTriggered() != len(tc.want) {
				t.Errorf("TimesTriggered = %d, want %d", trig.TimesTriggered(), len(tc.want))
			}
		})
	}
}

func TestCalendarIntervalTriggerMonthNormalization(t *testing.T) {
	// Jan 31 + 1 month has no Feb 31, so AddDate normalizes into March.
	start := mustTime(t, "2026-01-31 00:00:00")
	trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalMonth, 1)
	got := collectCalendarIntervalTriggerFires(trig, start, 3)
	want := []time.Time{
		mustTime(t, "2026-01-31 00:00:00"),
		mustTime(t, "2026-03-03 00:00:00"), // Feb 31 -> Mar 3 (2026 is not a leap year)
		mustTime(t, "2026-04-03 00:00:00"),
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("fire %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCalendarIntervalTriggerLeapYear(t *testing.T) {
	// Feb 29 + 1 year has no Feb 29 in the following year, so it normalizes.
	start := mustTime(t, "2024-02-29 12:00:00")
	trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalYear, 1)
	got := collectCalendarIntervalTriggerFires(trig, start, 2)
	want := []time.Time{
		mustTime(t, "2024-02-29 12:00:00"),
		mustTime(t, "2025-03-01 12:00:00"), // Feb 29 2025 -> Mar 1
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("fire %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCalendarIntervalTriggerEndTime(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	end := mustTime(t, "2026-01-03 12:00:00")
	trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
		WithEndTime(end)
	got := collectCalendarIntervalTriggerFires(trig, start, 100)
	// Fires on Jan 1, 2 and 3; Jan 4 is past the Jan 3 12:00 end.
	want := []time.Time{
		mustTime(t, "2026-01-01 00:00:00"),
		mustTime(t, "2026-01-02 00:00:00"),
		mustTime(t, "2026-01-03 00:00:00"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d fires, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("fire %d = %v, want %v", i, got[i], want[i])
		}
	}
	if trig.WillFireAgain() {
		t.Errorf("expected exhausted trigger, next = %v", trig.NextFireTime())
	}
	if !trig.PreviousFireTime().Equal(mustTime(t, "2026-01-03 00:00:00")) {
		t.Errorf("prev = %v, want 2026-01-03 00:00:00", trig.PreviousFireTime())
	}
}

func TestCalendarIntervalTriggerComputeFirstFireTime(t *testing.T) {
	start := mustTime(t, "2026-06-01 00:00:00")

	t.Run("returns_start_even_when_now_is_later", func(t *testing.T) {
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1)
		first := trig.ComputeFirstFireTime(start.Add(365 * 24 * time.Hour))
		if !first.Equal(start) {
			t.Fatalf("first = %v, want %v", first, start)
		}
	})

	t.Run("zero_when_start_past_end", func(t *testing.T) {
		end := start.Add(-time.Hour)
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
			WithEndTime(end)
		first := trig.ComputeFirstFireTime(start)
		if !first.IsZero() {
			t.Fatalf("first = %v, want zero", first)
		}
		if trig.WillFireAgain() {
			t.Fatalf("expected no fire, next = %v", trig.NextFireTime())
		}
	})
}

func TestCalendarIntervalTriggerNonPositiveCountFiresOnce(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	for _, count := range []int{0, -3} {
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, count)
		got := collectCalendarIntervalTriggerFires(trig, start, 10)
		if len(got) != 1 || !got[0].Equal(start) {
			t.Fatalf("count=%d: got %v, want single fire at %v", count, got, start)
		}
	}
}

func TestCalendarIntervalTriggerMisfire(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")

	t.Run("ignore_leaves_next_unchanged", func(t *testing.T) {
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
			WithMisfirePolicy(MisfireIgnore)
		trig.ComputeFirstFireTime(start)
		now := start.Add(10 * 24 * time.Hour)
		if got := trig.UpdateAfterMisfire(now); !got.Equal(start) {
			t.Fatalf("misfire next = %v, want %v (unchanged)", got, start)
		}
	})

	t.Run("fire_now_sets_now", func(t *testing.T) {
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
			WithMisfirePolicy(MisfireFireNow)
		trig.ComputeFirstFireTime(start)
		now := start.Add(90 * time.Hour)
		if got := trig.UpdateAfterMisfire(now); !got.Equal(now) {
			t.Fatalf("misfire next = %v, want %v (now)", got, now)
		}
	})

	t.Run("do_nothing_advances_past_now", func(t *testing.T) {
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
			WithMisfirePolicy(MisfireDoNothing)
		trig.ComputeFirstFireTime(start)
		// Three and a half days elapsed: next whole-day step after now is Jan 4.
		now := mustTime(t, "2026-01-03 12:00:00")
		got := trig.UpdateAfterMisfire(now)
		want := mustTime(t, "2026-01-04 00:00:00")
		if !got.Equal(want) {
			t.Fatalf("misfire next = %v, want %v", got, want)
		}
	})

	t.Run("smart_stops_at_end_time", func(t *testing.T) {
		end := mustTime(t, "2026-01-02 06:00:00")
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
			WithEndTime(end).
			WithMisfirePolicy(MisfireSmart)
		trig.ComputeFirstFireTime(start)
		now := mustTime(t, "2026-06-01 00:00:00")
		if got := trig.UpdateAfterMisfire(now); !got.IsZero() {
			t.Fatalf("misfire next = %v, want zero (past end)", got)
		}
	})
}

func TestCalendarIntervalTriggerBuilders(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	loc := time.FixedZone("X", 3600)
	end := start.Add(48 * time.Hour)
	trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 2).
		In(loc).
		WithEndTime(end).
		WithDescription("desc").
		WithMisfirePolicy(MisfireDoNothing).
		PreserveWallClock(true)

	if trig.Key() != NewKey("t") {
		t.Errorf("Key = %v", trig.Key())
	}
	if trig.JobKey() != NewKey("j") {
		t.Errorf("JobKey = %v", trig.JobKey())
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
	if trig.Unit() != IntervalDay || trig.Count() != 2 {
		t.Errorf("Unit/Count = %v/%d", trig.Unit(), trig.Count())
	}
	if !trig.StartTime().Equal(start) || !trig.EndTime().Equal(end) {
		t.Errorf("StartTime/EndTime = %v/%v", trig.StartTime(), trig.EndTime())
	}

	// A nil location is ignored, leaving the previously set location in place.
	if trig.In(nil).Location() != loc {
		t.Errorf("In(nil) changed location to %v", trig.Location())
	}
}

func TestCalendarIntervalTriggerDaylightSaving(t *testing.T) {
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("time zone data unavailable: %v", err)
	}
	// 2026-03-08 spring forward: 02:00 EST -> 03:00 EDT (offset -5h -> -4h).
	start := time.Date(2026, time.March, 7, 12, 0, 0, 0, nyc)

	t.Run("preserve_wall_clock_keeps_local_hour", func(t *testing.T) {
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
			In(nyc).
			PreserveWallClock(true)
		trig.ComputeFirstFireTime(start)
		trig.Triggered(trig.NextFireTime())
		next := trig.NextFireTime().In(nyc)
		if next.Hour() != 12 || next.Day() != 8 {
			t.Fatalf("next = %v, want 2026-03-08 12:00 local", next)
		}
	})

	t.Run("no_preserve_keeps_absolute_time_of_day", func(t *testing.T) {
		trig := NewCalendarIntervalTrigger(NewKey("t"), NewKey("j"), start, IntervalDay, 1).
			In(nyc).
			PreserveWallClock(false)
		trig.ComputeFirstFireTime(start)
		trig.Triggered(trig.NextFireTime())
		next := trig.NextFireTime()
		// Start is 17:00 UTC; preserving the absolute time-of-day yields
		// 17:00 UTC the next day, which is 13:00 EDT.
		wantUTC := time.Date(2026, time.March, 8, 17, 0, 0, 0, time.UTC)
		if !next.Equal(wantUTC) {
			t.Fatalf("next = %v, want %v", next.UTC(), wantUTC)
		}
		if local := next.In(nyc); local.Hour() != 13 {
			t.Fatalf("next local = %v, want 13:00 EDT", local)
		}
	})
}
