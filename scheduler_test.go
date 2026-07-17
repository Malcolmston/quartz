package quartz

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testClock is a manually advanced clock for deterministic tests.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(t time.Time) *testClock { return &testClock{now: t} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// countingJob records how many times it ran.
type countingJob struct{ n int32 }

func (j *countingJob) Execute(ctx context.Context) error {
	atomic.AddInt32(&j.n, 1)
	return nil
}

func (j *countingJob) count() int { return int(atomic.LoadInt32(&j.n)) }

func newTestScheduler(t time.Time) (*Scheduler, *testClock) {
	clk := newTestClock(t)
	s := NewScheduler(Options{
		Concurrency:      1,
		Clock:            clk.Now,
		MisfireThreshold: time.Minute,
	})
	return s, clk
}

func TestScheduleAndProcessDue(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, clk := newTestScheduler(start)

	job := &countingJob{}
	detail := NewJobDetail(NewKey("j"), job).WithDescription("counter")
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 2)

	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}

	// At start time exactly one fire is due.
	if fired := s.ProcessDue(); fired != 1 {
		t.Fatalf("fired = %d, want 1", fired)
	}
	if job.count() != 1 {
		t.Fatalf("count = %d, want 1", job.count())
	}

	// Nothing new is due yet.
	if fired := s.ProcessDue(); fired != 0 {
		t.Fatalf("fired = %d, want 0", fired)
	}

	// Advance one interval at a time (staying within the misfire threshold)
	// so each remaining fire is delivered.
	clk.Advance(time.Minute)
	s.ProcessDue()
	clk.Advance(time.Minute)
	s.ProcessDue()
	if job.count() != 3 {
		t.Fatalf("count = %d, want 3", job.count())
	}

	// Trigger should now be complete.
	st, err := s.Store().GetTrigger(trig.Key())
	if err != nil {
		t.Fatal(err)
	}
	if st.State != TriggerStateComplete {
		t.Fatalf("state = %v, want COMPLETE", st.State)
	}
}

func TestScheduleJobKeyMismatch(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	detail := NewJobDetail(NewKey("j"), &countingJob{})
	trig := NewSimpleTrigger(NewKey("t"), NewKey("other"), start, time.Minute, 0)
	if err := s.ScheduleJob(detail, trig); err == nil {
		t.Fatal("expected key mismatch error")
	}
}

func TestPauseResumeTrigger(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, clk := newTestScheduler(start)
	job := &countingJob{}
	detail := NewJobDetail(NewKey("j"), job)
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, RepeatForever)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}

	if err := s.PauseTrigger(trig.Key()); err != nil {
		t.Fatal(err)
	}
	clk.Advance(30 * time.Second)
	if fired := s.ProcessDue(); fired != 0 {
		t.Fatalf("paused trigger fired %d times", fired)
	}
	if job.count() != 0 {
		t.Fatalf("count = %d, want 0 while paused", job.count())
	}

	if err := s.ResumeTrigger(trig.Key()); err != nil {
		t.Fatal(err)
	}
	if fired := s.ProcessDue(); fired != 1 {
		t.Fatalf("fired = %d, want 1 after resume", fired)
	}
}

func TestPauseResumeJob(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	job := &countingJob{}
	detail := NewJobDetail(NewKey("j"), job)
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, RepeatForever)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseJob(detail.Key()); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Store().GetTrigger(trig.Key())
	if st.State != TriggerStatePaused {
		t.Fatalf("state = %v, want PAUSED", st.State)
	}
	if err := s.ResumeJob(detail.Key()); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Store().GetTrigger(trig.Key())
	if st.State != TriggerStateNormal {
		t.Fatalf("state = %v, want NORMAL", st.State)
	}
}

func TestUnscheduleRemovesNonDurableJob(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	detail := NewJobDetail(NewKey("j"), &countingJob{})
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 0)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	removed, err := s.UnscheduleJob(trig.Key())
	if err != nil || !removed {
		t.Fatalf("unschedule removed=%v err=%v", removed, err)
	}
	if _, err := s.Store().GetJob(detail.Key()); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected job removed, err=%v", err)
	}
}

func TestDurableJobSurvivesUnschedule(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	detail := NewJobDetail(NewKey("j"), &countingJob{}).Durable(true)
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 0)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UnscheduleJob(trig.Key()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store().GetJob(detail.Key()); err != nil {
		t.Fatalf("durable job should survive, err=%v", err)
	}
}

func TestTriggerListenerVeto(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	job := &countingJob{}
	detail := NewJobDetail(NewKey("j"), job)
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 0)

	vl := &vetoListener{}
	s.AddTriggerListener(vl)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	if fired := s.ProcessDue(); fired != 0 {
		t.Fatalf("vetoed fire reported %d", fired)
	}
	if job.count() != 0 {
		t.Fatalf("vetoed job executed %d times", job.count())
	}
	if !vl.fired || !vl.completed {
		t.Fatalf("listener callbacks fired=%v completed=%v", vl.fired, vl.completed)
	}
}

type vetoListener struct {
	BaseTriggerListener
	fired     bool
	completed bool
}

func (l *vetoListener) TriggerFired(context.Context, Trigger, *JobExecutionContext) { l.fired = true }
func (l *vetoListener) VetoJobExecution(context.Context, Trigger, *JobExecutionContext) bool {
	return true
}
func (l *vetoListener) TriggerComplete(context.Context, Trigger, *JobExecutionContext) {
	l.completed = true
}

func TestJobListenerObservesResult(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)

	wantErr := errors.New("boom")
	detail := NewJobDetail(NewKey("j"), JobFunc(func(context.Context) error { return wantErr }))
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, 0)

	jl := &recordingJobListener{}
	s.AddJobListener(jl)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	s.ProcessDue()
	if !jl.before || !jl.after {
		t.Fatalf("job listener before=%v after=%v", jl.before, jl.after)
	}
	if !errors.Is(jl.result, wantErr) {
		t.Fatalf("result = %v, want %v", jl.result, wantErr)
	}
}

type recordingJobListener struct {
	BaseJobListener
	before bool
	after  bool
	result error
}

func (l *recordingJobListener) JobToBeExecuted(context.Context, *JobExecutionContext) {
	l.before = true
}
func (l *recordingJobListener) JobWasExecuted(_ context.Context, jec *JobExecutionContext) {
	l.after = true
	l.result = jec.Result
}

func TestMisfireDoNothingSkipsMissed(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, clk := newTestScheduler(start)
	job := &countingJob{}
	detail := NewJobDetail(NewKey("j"), job)
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, time.Minute, RepeatForever).
		WithMisfirePolicy(MisfireDoNothing)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	// Jump far past the threshold without processing.
	clk.Advance(10 * time.Minute)
	s.ProcessDue()
	// DoNothing skips the missed fires and reschedules for the future, so no
	// execution occurs on this pass.
	if job.count() != 0 {
		t.Fatalf("count = %d, want 0 (missed fires skipped)", job.count())
	}
	next := trig.NextFireTime()
	if !next.After(clk.Now()) {
		t.Fatalf("next fire %v should be after now %v", next, clk.Now())
	}
}

func TestStartShutdownGracefulDrain(t *testing.T) {
	start := time.Now()
	var mu sync.Mutex
	var ran int
	done := make(chan struct{})
	s := NewScheduler(Options{
		Concurrency:  2,
		PollInterval: 5 * time.Millisecond,
	})
	detail := NewJobDetail(NewKey("j"), JobFunc(func(context.Context) error {
		mu.Lock()
		ran++
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	}))
	trig := NewSimpleTrigger(NewKey("t"), detail.Key(), start, 5*time.Millisecond, RepeatForever)
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	// Second Start is a no-op.
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if !s.IsStarted() {
		t.Fatal("scheduler should be started")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job never ran")
	}
	s.Shutdown(true)
	if s.IsStarted() {
		t.Fatal("scheduler should be stopped")
	}
	mu.Lock()
	got := ran
	mu.Unlock()
	if got == 0 {
		t.Fatal("expected at least one run")
	}
	// Operations after shutdown are rejected.
	if err := s.ScheduleJob(detail, trig); !errors.Is(err, ErrSchedulerShutdown) {
		t.Fatalf("expected shutdown error, got %v", err)
	}
}

func TestTriggerJobOutOfBand(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")
	s, _ := newTestScheduler(start)
	job := &countingJob{}
	detail := NewJobDetail(NewKey("j"), job).Durable(true)
	if err := s.AddJob(detail); err != nil {
		t.Fatal(err)
	}
	if err := s.TriggerJob(detail.Key()); err != nil {
		t.Fatal(err)
	}
	if job.count() != 1 {
		t.Fatalf("count = %d, want 1", job.count())
	}
	if err := s.TriggerJob(NewKey("missing")); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected job-not-found, got %v", err)
	}
}

func TestAddJobRequiresDurable(t *testing.T) {
	s, _ := newTestScheduler(mustTime(t, "2026-01-01 00:00:00"))
	detail := NewJobDetail(NewKey("j"), &countingJob{})
	if err := s.AddJob(detail); err == nil {
		t.Fatal("expected error adding non-durable job without trigger")
	}
}

func TestCronTriggerViaScheduler(t *testing.T) {
	start := mustTime(t, "2026-01-01 09:59:30")
	s, clk := newTestScheduler(start)
	job := &countingJob{}
	detail := NewJobDetail(NewKey("j"), job)
	trig, err := NewCronTriggerWithKeys(NewKey("t"), detail.Key(), "0 0 10 * * *")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ScheduleJob(detail, trig); err != nil {
		t.Fatal(err)
	}
	first := trig.NextFireTime()
	if !first.Equal(mustTime(t, "2026-01-01 10:00:00")) {
		t.Fatalf("first fire = %v", first)
	}
	if fired := s.ProcessDue(); fired != 0 {
		t.Fatalf("nothing should be due yet, fired %d", fired)
	}
	clk.Set(mustTime(t, "2026-01-01 10:00:00"))
	if fired := s.ProcessDue(); fired != 1 {
		t.Fatalf("fired = %d, want 1", fired)
	}
	if job.count() != 1 {
		t.Fatalf("count = %d, want 1", job.count())
	}
}
