package quartz

import (
	"testing"
	"time"
)

func TestSimpleTriggerBoundedRepeat(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Minute, 2)

	first := trig.ComputeFirstFireTime(start.Add(-time.Hour))
	if !first.Equal(start) {
		t.Fatalf("first = %v, want %v", first, start)
	}

	// repeatCount=2 means 3 total fires: 00:00, 00:01, 00:02.
	var fires []time.Time
	for trig.WillFireAgain() {
		fires = append(fires, trig.NextFireTime())
		trig.Triggered(trig.NextFireTime())
	}
	want := []time.Time{
		start,
		start.Add(time.Minute),
		start.Add(2 * time.Minute),
	}
	if len(fires) != len(want) {
		t.Fatalf("got %d fires, want %d: %v", len(fires), len(want), fires)
	}
	for i := range want {
		if !fires[i].Equal(want[i]) {
			t.Errorf("fire %d = %v, want %v", i, fires[i], want[i])
		}
	}
	if trig.TimesTriggered() != 3 {
		t.Errorf("TimesTriggered = %d, want 3", trig.TimesTriggered())
	}
}

func TestSimpleTriggerRepeatForever(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Hour, RepeatForever)
	trig.ComputeFirstFireTime(start)
	for i := 0; i < 1000; i++ {
		if !trig.WillFireAgain() {
			t.Fatalf("stopped firing after %d iterations", i)
		}
		trig.Triggered(trig.NextFireTime())
	}
	want := start.Add(1000 * time.Hour)
	if !trig.NextFireTime().Equal(want) {
		t.Fatalf("next = %v, want %v", trig.NextFireTime(), want)
	}
}

func TestSimpleTriggerEndTime(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	end := start.Add(90 * time.Minute)
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Hour, RepeatForever).
		WithEndTime(end)
	trig.ComputeFirstFireTime(start)

	var count int
	for trig.WillFireAgain() {
		count++
		trig.Triggered(trig.NextFireTime())
		if count > 10 {
			t.Fatal("did not terminate at end time")
		}
	}
	// Fires at 00:00 and 01:00; 02:00 is past the 01:30 end.
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestSimpleTriggerSingleFire(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, 0, 0)
	trig.ComputeFirstFireTime(start)
	if !trig.NextFireTime().Equal(start) {
		t.Fatalf("next = %v, want %v", trig.NextFireTime(), start)
	}
	trig.Triggered(start)
	if trig.WillFireAgain() {
		t.Fatalf("expected exhausted trigger, next = %v", trig.NextFireTime())
	}
	if !trig.PreviousFireTime().Equal(start) {
		t.Fatalf("prev = %v, want %v", trig.PreviousFireTime(), start)
	}
}

func TestSimpleTriggerMisfireDoNothing(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Hour, RepeatForever).
		WithMisfirePolicy(MisfireDoNothing)
	trig.ComputeFirstFireTime(start)

	// Pretend three hours elapsed with no fires.
	now := start.Add(3*time.Hour + 30*time.Minute)
	next := trig.UpdateAfterMisfire(now)
	want := start.Add(4 * time.Hour)
	if !next.Equal(want) {
		t.Fatalf("misfire next = %v, want %v", next, want)
	}
}

func TestSimpleTriggerMisfireFireNow(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Hour, RepeatForever).
		WithMisfirePolicy(MisfireFireNow)
	trig.ComputeFirstFireTime(start)
	now := start.Add(3 * time.Hour)
	next := trig.UpdateAfterMisfire(now)
	if !next.Equal(now) {
		t.Fatalf("misfire next = %v, want %v (now)", next, now)
	}
}
