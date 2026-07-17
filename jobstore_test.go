package quartz

import (
	"errors"
	"testing"
	"time"
)

func TestMemoryJobStoreJobLifecycle(t *testing.T) {
	store := NewMemoryJobStore()
	detail := NewJobDetail(NewKey("j"), &countingJob{})

	if err := store.StoreJob(detail, false); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreJob(detail, false); !errors.Is(err, ErrJobExists) {
		t.Fatalf("expected ErrJobExists, got %v", err)
	}
	if err := store.StoreJob(detail, true); err != nil {
		t.Fatalf("replace should succeed: %v", err)
	}
	got, err := store.GetJob(detail.Key())
	if err != nil || got != detail {
		t.Fatalf("GetJob = %v, %v", got, err)
	}
	if keys := store.JobKeys(); len(keys) != 1 {
		t.Fatalf("JobKeys = %v", keys)
	}
	removed, _ := store.RemoveJob(detail.Key())
	if !removed {
		t.Fatal("expected removal")
	}
	if _, err := store.GetJob(detail.Key()); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}

func TestMemoryJobStoreTriggerLifecycle(t *testing.T) {
	store := NewMemoryJobStore()
	start := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Minute, 0)
	trig.ComputeFirstFireTime(start)

	if err := store.StoreTrigger(trig, TriggerStateNormal, false); err != nil {
		t.Fatal(err)
	}
	if err := store.StoreTrigger(trig, TriggerStateNormal, false); !errors.Is(err, ErrTriggerExists) {
		t.Fatalf("expected ErrTriggerExists, got %v", err)
	}
	if err := store.SetTriggerState(trig.Key(), TriggerStatePaused); err != nil {
		t.Fatal(err)
	}
	st, err := store.GetTrigger(trig.Key())
	if err != nil || st.State != TriggerStatePaused {
		t.Fatalf("GetTrigger = %v, %v", st, err)
	}
	if err := store.SetTriggerState(NewKey("missing"), TriggerStateNormal); !errors.Is(err, ErrTriggerNotFound) {
		t.Fatalf("expected ErrTriggerNotFound, got %v", err)
	}
	if forJob := store.TriggersForJob(NewKey("j")); len(forJob) != 1 {
		t.Fatalf("TriggersForJob = %v", forJob)
	}
	if keys := store.TriggerKeys(); len(keys) != 1 {
		t.Fatalf("TriggerKeys = %v", keys)
	}
	removed, _ := store.RemoveTrigger(trig.Key())
	if !removed {
		t.Fatal("expected removal")
	}
	if _, err := store.GetTrigger(trig.Key()); !errors.Is(err, ErrTriggerNotFound) {
		t.Fatalf("expected ErrTriggerNotFound, got %v", err)
	}
}

func TestAcquireNextTriggersOrdering(t *testing.T) {
	store := NewMemoryJobStore()
	base := mustTime(t, "2026-01-01 00:00:00")

	// Three triggers with staggered fire times, inserted out of order.
	for _, off := range []time.Duration{2 * time.Minute, 0, time.Minute} {
		key := NewKey("t" + off.String())
		trig := NewSimpleTrigger(key, NewKey("j"), base.Add(off), time.Hour, 0)
		trig.ComputeFirstFireTime(base)
		if err := store.StoreTrigger(trig, TriggerStateNormal, false); err != nil {
			t.Fatal(err)
		}
	}
	// A paused trigger should be excluded.
	paused := NewSimpleTrigger(NewKey("paused"), NewKey("j"), base, time.Hour, 0)
	paused.ComputeFirstFireTime(base)
	if err := store.StoreTrigger(paused, TriggerStatePaused, false); err != nil {
		t.Fatal(err)
	}

	got := store.AcquireNextTriggers(base.Add(90*time.Second), 0)
	if len(got) != 2 {
		t.Fatalf("acquired %d, want 2 (paused excluded, third too late)", len(got))
	}
	if !got[0].Trigger.NextFireTime().Before(got[1].Trigger.NextFireTime()) {
		t.Fatal("results not ordered by fire time")
	}

	// maxCount limits results.
	limited := store.AcquireNextTriggers(base.Add(time.Hour), 1)
	if len(limited) != 1 {
		t.Fatalf("maxCount ignored, got %d", len(limited))
	}
}
