package quartz

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Common scheduler errors.
var (
	// ErrSchedulerRunning is returned by operations that are not allowed
	// while the scheduler is running.
	ErrSchedulerRunning = errors.New("quartz: scheduler is running")
	// ErrSchedulerShutdown is returned by operations attempted after the
	// scheduler has been shut down.
	ErrSchedulerShutdown = errors.New("quartz: scheduler is shut down")
)

// Options configures a Scheduler.
type Options struct {
	// Store is the JobStore to use. Defaults to a new MemoryJobStore.
	Store JobStore
	// Concurrency is the number of worker goroutines. Defaults to 1.
	Concurrency int
	// Clock returns the current time. Defaults to time.Now. Injecting a
	// custom clock makes firing decisions deterministic in tests.
	Clock func() time.Time
	// PollInterval is how often the background loop wakes to check for due
	// triggers when running in real time. Defaults to 100ms.
	PollInterval time.Duration
	// MisfireThreshold is how late a trigger may fire before it is treated
	// as a misfire. Defaults to 5s.
	MisfireThreshold time.Duration
}

// Scheduler schedules jobs, fires their triggers, and runs the resulting work
// on a bounded worker pool. It is safe for concurrent use.
type Scheduler struct {
	store            JobStore
	concurrency      int
	now              func() time.Time
	pollInterval     time.Duration
	misfireThreshold time.Duration

	mu         sync.Mutex
	started    bool
	shutdown   bool
	workClosed bool

	work    chan *JobExecutionContext
	workers sync.WaitGroup
	loopWG  sync.WaitGroup
	quit    chan struct{}
	signal  chan struct{}
	baseCtx context.Context
	cancel  context.CancelFunc

	jobListeners     []JobListener
	triggerListeners []TriggerListener
}

// NewScheduler constructs a Scheduler from Options, applying defaults for any
// zero valued field.
func NewScheduler(opts Options) *Scheduler {
	if opts.Store == nil {
		opts.Store = NewMemoryJobStore()
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 100 * time.Millisecond
	}
	if opts.MisfireThreshold <= 0 {
		opts.MisfireThreshold = 5 * time.Second
	}
	return &Scheduler{
		store:            opts.Store,
		concurrency:      opts.Concurrency,
		now:              opts.Clock,
		pollInterval:     opts.PollInterval,
		misfireThreshold: opts.MisfireThreshold,
		quit:             make(chan struct{}),
		signal:           make(chan struct{}, 1),
	}
}

// Store returns the underlying JobStore.
func (s *Scheduler) Store() JobStore { return s.store }

// Now returns the scheduler's current time using its injected clock.
func (s *Scheduler) Now() time.Time { return s.now() }

// AddJobListener registers a JobListener. It must be called before Start.
func (s *Scheduler) AddJobListener(l JobListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobListeners = append(s.jobListeners, l)
}

// AddTriggerListener registers a TriggerListener. It must be called before
// Start.
func (s *Scheduler) AddTriggerListener(l TriggerListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggerListeners = append(s.triggerListeners, l)
}

// ScheduleJob stores the job and its trigger and computes the trigger's first
// fire time. If detail is nil the job is assumed to already exist in the store.
func (s *Scheduler) ScheduleJob(detail *JobDetail, trigger Trigger) error {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return ErrSchedulerShutdown
	}
	s.mu.Unlock()

	if detail != nil {
		if trigger.JobKey() != detail.Key() {
			return fmt.Errorf("quartz: trigger job key %s does not match job %s", trigger.JobKey(), detail.Key())
		}
		if err := s.store.StoreJob(detail, true); err != nil {
			return err
		}
	} else {
		if _, err := s.store.GetJob(trigger.JobKey()); err != nil {
			return err
		}
	}

	first := trigger.ComputeFirstFireTime(s.now())
	state := TriggerStateNormal
	if first.IsZero() {
		state = TriggerStateComplete
	}
	if err := s.store.StoreTrigger(trigger, state, true); err != nil {
		return err
	}
	s.wake()
	return nil
}

// AddJob stores a durable job without any trigger.
func (s *Scheduler) AddJob(detail *JobDetail) error {
	if !detail.IsDurable() {
		return errors.New("quartz: only durable jobs may be added without a trigger")
	}
	return s.store.StoreJob(detail, true)
}

// UnscheduleJob removes a trigger. If the referenced job is not durable and has
// no remaining triggers, the job is removed as well.
func (s *Scheduler) UnscheduleJob(triggerKey Key) (bool, error) {
	st, err := s.store.GetTrigger(triggerKey)
	if err != nil {
		return false, err
	}
	jobKey := st.Trigger.JobKey()
	removed, err := s.store.RemoveTrigger(triggerKey)
	if err != nil || !removed {
		return removed, err
	}
	if remaining := s.store.TriggersForJob(jobKey); len(remaining) == 0 {
		if job, err := s.store.GetJob(jobKey); err == nil && !job.IsDurable() {
			if _, err := s.store.RemoveJob(jobKey); err != nil {
				return removed, err
			}
		}
	}
	return removed, nil
}

// DeleteJob removes a job and all of its triggers.
func (s *Scheduler) DeleteJob(jobKey Key) (bool, error) {
	for _, st := range s.store.TriggersForJob(jobKey) {
		if _, err := s.store.RemoveTrigger(st.Trigger.Key()); err != nil {
			return false, err
		}
	}
	return s.store.RemoveJob(jobKey)
}

// PauseTrigger sets a trigger to the paused state so it will not fire until
// resumed.
func (s *Scheduler) PauseTrigger(key Key) error {
	return s.store.SetTriggerState(key, TriggerStatePaused)
}

// ResumeTrigger returns a paused trigger to the normal state, applying misfire
// handling for any fires missed while paused.
func (s *Scheduler) ResumeTrigger(key Key) error {
	st, err := s.store.GetTrigger(key)
	if err != nil {
		return err
	}
	if st.State != TriggerStatePaused {
		return nil
	}
	s.applyMisfireIfNeeded(st.Trigger)
	state := TriggerStateNormal
	if !st.Trigger.WillFireAgain() {
		state = TriggerStateComplete
	}
	if err := s.store.SetTriggerState(key, state); err != nil {
		return err
	}
	s.wake()
	return nil
}

// PauseJob pauses all triggers of a job.
func (s *Scheduler) PauseJob(jobKey Key) error {
	for _, st := range s.store.TriggersForJob(jobKey) {
		if err := s.PauseTrigger(st.Trigger.Key()); err != nil {
			return err
		}
	}
	return nil
}

// ResumeJob resumes all triggers of a job.
func (s *Scheduler) ResumeJob(jobKey Key) error {
	for _, st := range s.store.TriggersForJob(jobKey) {
		if err := s.ResumeTrigger(st.Trigger.Key()); err != nil {
			return err
		}
	}
	return nil
}

// TriggerJob fires a job immediately, out of band from its triggers, by
// dispatching a one-off execution. The job must exist in the store.
func (s *Scheduler) TriggerJob(jobKey Key) error {
	detail, err := s.store.GetJob(jobKey)
	if err != nil {
		return err
	}
	jec := &JobExecutionContext{
		Scheduler:  s,
		Detail:     detail,
		FireTime:   s.now(),
		MergedData: detail.Data().Clone(),
	}
	s.dispatch(jec)
	return nil
}

// Start launches the worker pool and the background scheduling loop. It is a
// no-op if the scheduler is already running and returns ErrSchedulerShutdown if
// it was shut down.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shutdown {
		return ErrSchedulerShutdown
	}
	if s.started {
		return nil
	}
	s.started = true
	s.work = make(chan *JobExecutionContext)
	s.baseCtx, s.cancel = context.WithCancel(context.Background())
	for i := 0; i < s.concurrency; i++ {
		s.workers.Add(1)
		go s.worker()
	}
	s.loopWG.Add(1)
	go s.loop()
	return nil
}

// Shutdown stops the scheduling loop and the worker pool. When waitForJobs is
// true it blocks until all in-flight and queued jobs have drained; otherwise it
// cancels running jobs' contexts and returns promptly.
func (s *Scheduler) Shutdown(waitForJobs bool) {
	s.mu.Lock()
	if !s.started || s.shutdown {
		s.shutdown = true
		s.mu.Unlock()
		return
	}
	s.shutdown = true
	s.mu.Unlock()

	close(s.quit)
	s.loopWG.Wait()

	if !waitForJobs {
		// Cancel running jobs' contexts so they can abort promptly.
		s.cancel()
	}

	// No more work will be dispatched; close the queue so workers exit
	// after finishing any job currently in hand.
	s.mu.Lock()
	s.workClosed = true
	close(s.work)
	s.mu.Unlock()

	s.workers.Wait()
}

// IsStarted reports whether the scheduler is running.
func (s *Scheduler) IsStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started && !s.shutdown
}

// wake nudges the background loop to re-evaluate due triggers.
func (s *Scheduler) wake() {
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// loop is the background scheduling goroutine.
func (s *Scheduler) loop() {
	defer s.loopWG.Done()
	timer := time.NewTimer(s.pollInterval)
	defer timer.Stop()
	for {
		s.ProcessDue()
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(s.pollInterval)
		select {
		case <-s.quit:
			return
		case <-s.signal:
		case <-timer.C:
		}
	}
}

// ProcessDue fires every trigger that is due at the scheduler's current time.
// It is called by the background loop and may also be invoked directly to drive
// the scheduler deterministically in tests using an injected clock. It returns
// the number of triggers fired.
func (s *Scheduler) ProcessDue() int {
	now := s.now()
	due := s.store.AcquireNextTriggers(now, 0)
	fired := 0
	for _, st := range due {
		trig := st.Trigger

		// Misfire detection: fire time is older than the threshold.
		if now.Sub(trig.NextFireTime()) > s.misfireThreshold {
			s.notifyMisfire(trig)
			trig.UpdateAfterMisfire(now)
			s.persistAfterAdvance(trig)
			if trig.NextFireTime().IsZero() || trig.NextFireTime().After(now) {
				continue
			}
		}

		if s.fireTrigger(trig) {
			fired++
		}
	}
	return fired
}

// fireTrigger builds the execution context, advances the trigger, and
// dispatches the job. It returns true if the job was dispatched (not vetoed).
func (s *Scheduler) fireTrigger(trig Trigger) bool {
	detail, err := s.store.GetJob(trig.JobKey())
	if err != nil {
		// Orphaned trigger: mark complete so it stops being acquired.
		_ = s.store.SetTriggerState(trig.Key(), TriggerStateComplete)
		return false
	}

	fireTime := trig.NextFireTime()
	jec := &JobExecutionContext{
		Scheduler:  s,
		Detail:     detail,
		Trigger:    trig,
		FireTime:   fireTime,
		MergedData: detail.Data().Clone(),
	}

	ctx := context.Background()
	vetoed := false
	for _, tl := range s.triggerListeners {
		tl.TriggerFired(ctx, trig, jec)
		if tl.VetoJobExecution(ctx, trig, jec) {
			vetoed = true
		}
	}

	// Advance the trigger regardless of veto so the schedule proceeds.
	trig.Triggered(s.now())
	s.persistAfterAdvance(trig)

	if vetoed {
		for _, tl := range s.triggerListeners {
			tl.TriggerComplete(ctx, trig, jec)
		}
		return false
	}

	s.dispatch(jec)
	return true
}

// persistAfterAdvance writes back the trigger's post-advance state, marking it
// complete when it will not fire again.
func (s *Scheduler) persistAfterAdvance(trig Trigger) {
	state := TriggerStateNormal
	if !trig.WillFireAgain() {
		state = TriggerStateComplete
	}
	_ = s.store.StoreTrigger(trig, state, true)
}

// applyMisfireIfNeeded advances a trigger past now if its scheduled fire time
// is already in the past, using the trigger's misfire policy.
func (s *Scheduler) applyMisfireIfNeeded(trig Trigger) {
	now := s.now()
	next := trig.NextFireTime()
	if next.IsZero() {
		return
	}
	if now.Sub(next) > s.misfireThreshold {
		trig.UpdateAfterMisfire(now)
	}
}

// dispatch sends a job execution to a worker, or runs it inline if the
// scheduler is not started (used by TriggerJob before Start and in tests).
func (s *Scheduler) dispatch(jec *JobExecutionContext) {
	s.mu.Lock()
	if !s.started || s.shutdown || s.workClosed {
		s.mu.Unlock()
		s.execute(context.Background(), jec)
		return
	}
	ch := s.work
	quit := s.quit
	s.mu.Unlock()

	select {
	case ch <- jec:
	case <-quit:
		// Shutting down: run inline so the fire is not silently lost.
		s.execute(context.Background(), jec)
	}
}

// worker consumes job executions until the work channel is closed.
func (s *Scheduler) worker() {
	defer s.workers.Done()
	for jec := range s.work {
		s.execute(s.baseCtx, jec)
	}
}

// execute runs a single job execution through its listeners.
func (s *Scheduler) execute(ctx context.Context, jec *JobExecutionContext) {
	for _, jl := range s.jobListeners {
		jl.JobToBeExecuted(ctx, jec)
	}
	jec.Result = jec.Detail.Job().Execute(ctx)
	for _, jl := range s.jobListeners {
		jl.JobWasExecuted(ctx, jec)
	}
	if jec.Trigger != nil {
		for _, tl := range s.triggerListeners {
			tl.TriggerComplete(ctx, jec.Trigger, jec)
		}
	}
}

// notifyMisfire is a hook point for future misfire listeners. It currently
// advances no state itself.
func (s *Scheduler) notifyMisfire(Trigger) {}
