// Package quartz is a Quartz-style, in-process job scheduler written entirely
// with the Go standard library. It provides jobs, triggers (simple interval and
// full cron), a worker pool, listeners, and a pluggable job store.
//
// # Overview
//
// The three central concepts are the Job, the JobDetail, and the Trigger:
//
//   - A Job is the code to run: any type implementing Execute(context.Context)
//     error. The JobFunc adapter turns a plain function into a Job.
//   - A JobDetail names a job (via a Key), attaches a description and a
//     JobDataMap of state, and holds the Job implementation. One JobDetail may
//     be fired by many triggers.
//   - A Trigger decides when a job fires. Two implementations are provided:
//     SimpleTrigger (a start time plus a fixed interval and an optional repeat
//     count) and CronTrigger (a full cron expression evaluated in a chosen time
//     zone).
//
// A Scheduler ties them together. It stores jobs and triggers in a JobStore,
// runs a background loop that fires due triggers, and executes the resulting
// work on a bounded pool of worker goroutines.
//
// # Cron expressions
//
// CronTrigger uses ParseCron, which understands six space separated fields
// (second, minute, hour, day-of-month, month, day-of-week); a five field
// expression without seconds is also accepted. Fields support ranges (1-5),
// steps (*/15 and 10-30/5), lists (1,3,5), names (JAN..DEC and SUN..SAT), and
// the * and ? wildcards. See CronExpression for the exact grammar and the day
// matching rules.
//
// # Deterministic time
//
// Every fire-time decision flows through an injectable clock. Triggers compute
// their next fire time as a pure function of a supplied "now", and the
// Scheduler reads time through Options.Clock. Tests can therefore drive the
// scheduler with a virtual clock and call Scheduler.ProcessDue directly,
// without sleeping or depending on the wall clock.
//
// # Lifecycle and control
//
// Jobs and triggers can be scheduled, unscheduled, paused, and resumed
// individually or by job. Start launches the worker pool and background loop;
// Shutdown stops them, optionally draining in-flight work. Triggers that fire
// late are reconciled through a configurable misfire policy.
//
// # Listeners
//
// JobListener and TriggerListener expose hooks around each execution:
// TriggerListener can even veto a job for a particular fire. Embed
// BaseJobListener or BaseTriggerListener to implement only the callbacks you
// need.
//
// # Concurrency
//
// The Scheduler, MemoryJobStore, and all exported scheduler methods are safe
// for concurrent use. Trigger implementations are mutated only by the scheduler
// and are not intended to be advanced from multiple goroutines simultaneously.
//
// # Example
//
// The following schedules a job to run every 15 seconds:
//
//	s := quartz.NewScheduler(quartz.Options{Concurrency: 4})
//	job := quartz.NewJobDetail(quartz.NewKey("report"), quartz.JobFunc(
//		func(ctx context.Context) error { return nil }))
//	trig := quartz.NewSimpleTrigger(
//		quartz.NewKey("report-trigger"), job.Key(),
//		time.Now(), 15*time.Second, quartz.RepeatForever)
//	_ = s.ScheduleJob(job, trig)
//	_ = s.Start()
//	defer s.Shutdown(true)
package quartz
