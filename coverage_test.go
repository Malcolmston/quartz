package quartz

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestKeyAndDataMap(t *testing.T) {
	k := NewKeyInGroup("name", "")
	if k.Group != DefaultGroup {
		t.Fatalf("empty group not defaulted: %q", k.Group)
	}
	k2 := NewKeyInGroup("name", "grp")
	if k2.String() != "grp.name" {
		t.Fatalf("String = %q", k2.String())
	}
	if (Key{}).IsZero() != true || k.IsZero() {
		t.Fatal("IsZero wrong")
	}

	data := JobDataMap{"s": "hello", "n": 7}
	clone := data.Clone()
	clone["s"] = "changed"
	if v, _ := data.GetString("s"); v != "hello" {
		t.Fatalf("clone leaked mutation: %q", v)
	}
	if v, ok := data.GetString("s"); !ok || v != "hello" {
		t.Fatalf("GetString = %q,%v", v, ok)
	}
	if _, ok := data.GetString("missing"); ok {
		t.Fatal("GetString missing should be false")
	}
	if _, ok := data.GetString("n"); ok {
		t.Fatal("GetString on int should be false")
	}
	if v, ok := data.GetInt("n"); !ok || v != 7 {
		t.Fatalf("GetInt = %d,%v", v, ok)
	}
	if _, ok := data.GetInt("missing"); ok {
		t.Fatal("GetInt missing should be false")
	}
	if _, ok := data.GetInt("s"); ok {
		t.Fatal("GetInt on string should be false")
	}
}

func TestJobDetailAccessors(t *testing.T) {
	d := NewJobDetail(NewKey("j"), JobFunc(func(context.Context) error { return nil })).
		WithDescription("desc").
		WithData(JobDataMap{"k": "v"})
	if d.Description() != "desc" {
		t.Fatal("Description")
	}
	if v, _ := d.Data().GetString("k"); v != "v" {
		t.Fatal("Data")
	}
	if !strings.Contains(d.String(), "JobDetail") {
		t.Fatalf("String = %q", d.String())
	}
}

func TestSimpleTriggerAccessors(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Minute, 5).
		WithDescription("every minute")
	if trig.Description() != "every minute" {
		t.Fatal("Description")
	}
	if trig.MisfirePolicy() != MisfireSmart {
		t.Fatal("default policy")
	}
	if !trig.StartTime().Equal(start) {
		t.Fatal("StartTime")
	}
	if trig.RepeatCount() != 5 {
		t.Fatal("RepeatCount")
	}
	if trig.Interval() != time.Minute {
		t.Fatal("Interval")
	}
	if !strings.Contains(trig.String(), "SimpleTrigger") {
		t.Fatalf("String = %q", trig.String())
	}
}

func TestSimpleTriggerMisfireIgnoreAndSingleFire(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	// Ignore policy leaves next untouched.
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Minute, RepeatForever).
		WithMisfirePolicy(MisfireIgnore)
	trig.ComputeFirstFireTime(start)
	before := trig.NextFireTime()
	if got := trig.UpdateAfterMisfire(start.Add(time.Hour)); !got.Equal(before) {
		t.Fatalf("ignore changed next: %v -> %v", before, got)
	}

	// Smart policy with zero interval and a past single fire clears next.
	single := NewSimpleTrigger(NewKey("t2"), NewKey("j"), start, 0, 0)
	single.ComputeFirstFireTime(start)
	if got := single.UpdateAfterMisfire(start.Add(time.Hour)); !got.IsZero() {
		t.Fatalf("expected cleared next, got %v", got)
	}
}

func TestSimpleTriggerStartAfterEndNeverFires(t *testing.T) {
	start := mustTime(t, "2026-01-02 00:00:00")
	end := mustTime(t, "2026-01-01 00:00:00")
	trig := NewSimpleTrigger(NewKey("t"), NewKey("j"), start, time.Minute, 0).WithEndTime(end)
	if got := trig.ComputeFirstFireTime(start); !got.IsZero() {
		t.Fatalf("expected no fire, got %v", got)
	}
	if trig.WillFireAgain() {
		t.Fatal("should not fire")
	}
}

func TestCronTriggerAccessors(t *testing.T) {
	trig, err := NewCronTrigger("t", "j", "0 0 12 * * *")
	if err != nil {
		t.Fatal(err)
	}
	trig.WithDescription("noon").
		WithMisfirePolicy(MisfireFireNow).
		StartingAt(mustTime(t, "2026-01-01 00:00:00")).
		EndingAt(mustTime(t, "2027-01-01 00:00:00"))
	if trig.Description() != "noon" {
		t.Fatal("Description")
	}
	if trig.MisfirePolicy() != MisfireFireNow {
		t.Fatal("MisfirePolicy")
	}
	if trig.Expression().String() != "0 0 12 * * *" {
		t.Fatal("Expression")
	}
	if trig.Key().Name != "t" || trig.JobKey().Name != "j" {
		t.Fatal("keys")
	}
	if !strings.Contains(trig.String(), "CronTrigger") {
		t.Fatalf("String = %q", trig.String())
	}

	// In(nil) is ignored.
	if trig.In(nil) != trig {
		t.Fatal("In(nil) should return trigger")
	}
}

func TestCronTriggerFireSequenceAndMisfire(t *testing.T) {
	trig, err := NewCronTrigger("t", "j", "0 0 12 * * *")
	if err != nil {
		t.Fatal(err)
	}
	now := mustTime(t, "2026-01-01 00:00:00")
	first := trig.ComputeFirstFireTime(now)
	if !first.Equal(mustTime(t, "2026-01-01 12:00:00")) {
		t.Fatalf("first = %v", first)
	}
	next := trig.Triggered(first)
	if !next.Equal(mustTime(t, "2026-01-02 12:00:00")) {
		t.Fatalf("next = %v", next)
	}
	if !trig.PreviousFireTime().Equal(first) {
		t.Fatalf("prev = %v", trig.PreviousFireTime())
	}

	// FireNow misfire sets next to now.
	trig.WithMisfirePolicy(MisfireFireNow)
	misNow := mustTime(t, "2026-01-05 08:00:00")
	if got := trig.UpdateAfterMisfire(misNow); !got.Equal(misNow) {
		t.Fatalf("FireNow misfire = %v, want %v", got, misNow)
	}

	// Ignore misfire leaves next unchanged.
	trig.WithMisfirePolicy(MisfireIgnore)
	keep := trig.NextFireTime()
	if got := trig.UpdateAfterMisfire(misNow.Add(time.Hour)); !got.Equal(keep) {
		t.Fatalf("Ignore misfire changed next: %v", got)
	}

	// Smart misfire advances past now.
	trig.WithMisfirePolicy(MisfireSmart)
	smartNow := mustTime(t, "2026-02-01 13:00:00")
	got := trig.UpdateAfterMisfire(smartNow)
	if !got.Equal(mustTime(t, "2026-02-02 12:00:00")) {
		t.Fatalf("Smart misfire = %v", got)
	}
}

func TestCronTriggerEndTimeStopsFiring(t *testing.T) {
	trig, err := NewCronTrigger("t", "j", "0 0 12 * * *")
	if err != nil {
		t.Fatal(err)
	}
	trig.EndingAt(mustTime(t, "2026-01-01 06:00:00"))
	if got := trig.ComputeFirstFireTime(mustTime(t, "2026-01-01 00:00:00")); !got.IsZero() {
		t.Fatalf("expected no fire past end, got %v", got)
	}
}

func TestDeleteJobRemovesTriggers(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	detail := NewJobDetail(NewKey("j"), &countingJob{}).Durable(true)
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 0)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	removed, err := s.DeleteJob(detail.Key())
	if err != nil || !removed {
		t.Fatalf("DeleteJob removed=%v err=%v", removed, err)
	}
	if _, err := s.Store().GetTrigger(trig.Key()); err == nil {
		t.Fatal("trigger should be gone")
	}
	if _, err := s.Store().GetJob(detail.Key()); err == nil {
		t.Fatal("job should be gone")
	}
}

func TestScheduleExistingJobWithoutDetail(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	detail := NewJobDetail(NewKey("j"), &countingJob{}).Durable(true)
	if err := s.AddJob(detail); err != nil {
		t.Fatal(err)
	}
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 0)
	// Passing nil detail references the already-stored job.
	if err := s.ScheduleJob(nil, trig); err != nil {
		t.Fatal(err)
	}
	// Referencing a missing job fails.
	orphan := NewSimpleTrigger(NewKey("t2"), NewKey("missing"), start, time.Minute, 0)
	if err := s.ScheduleJob(nil, orphan); err == nil {
		t.Fatal("expected error for missing job")
	}
}

func TestSchedulerNowAndTriggerStateString(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	if !s.Now().Equal(start) {
		t.Fatalf("Now = %v", s.Now())
	}
	if TriggerStateNormal.String() != "NORMAL" ||
		TriggerStatePaused.String() != "PAUSED" ||
		TriggerStateComplete.String() != "COMPLETE" {
		t.Fatal("TriggerState.String")
	}
}

func TestOrphanTriggerMarkedComplete(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	// Store a trigger whose job does not exist directly in the store.
	trig := NewSimpleTrigger(NewKey("t"), NewKey("ghost"), start, time.Minute, 0)
	trig.ComputeFirstFireTime(start)
	if err := s.Store().StoreTrigger(trig, TriggerStateNormal, false); err != nil {
		t.Fatal(err)
	}
	if fired := s.ProcessDue(); fired != 0 {
		t.Fatalf("orphan trigger fired %d", fired)
	}
	st, _ := s.Store().GetTrigger(trig.Key())
	if st.State != TriggerStateComplete {
		t.Fatalf("orphan state = %v, want COMPLETE", st.State)
	}
}

func TestBaseListenersAreNoOps(t *testing.T) {
	ctx := context.Background()
	jl := BaseJobListener{ListenerName: "jl"}
	if jl.Name() != "jl" {
		t.Fatal("Name")
	}
	jl.JobToBeExecuted(ctx, nil)
	jl.JobWasExecuted(ctx, nil)

	tl := BaseTriggerListener{ListenerName: "tl"}
	if tl.Name() != "tl" {
		t.Fatal("Name")
	}
	tl.TriggerFired(ctx, nil, nil)
	if tl.VetoJobExecution(ctx, nil, nil) {
		t.Fatal("base should not veto")
	}
	tl.TriggerComplete(ctx, nil, nil)
}

func TestResumeNonPausedTriggerNoop(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	detail := NewJobDetail(NewKey("j"), &countingJob{})
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 0)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	// Resuming a NORMAL trigger is a no-op and must not error.
	if err := s.ResumeTrigger(trig.Key()); err != nil {
		t.Fatal(err)
	}
}
