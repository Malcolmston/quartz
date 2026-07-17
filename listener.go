package quartz

import (
	"context"
	"time"
)

// JobExecutionContext carries the state of a single job execution as it flows
// through listeners and the Job itself.
type JobExecutionContext struct {
	// Scheduler is the scheduler running the job.
	Scheduler *Scheduler
	// Detail is the JobDetail being executed.
	Detail *JobDetail
	// Trigger is the trigger that caused this execution.
	Trigger Trigger
	// FireTime is the scheduled fire time for this execution.
	FireTime time.Time
	// MergedData is the JobDataMap available to the job (a clone of the
	// job's data).
	MergedData JobDataMap
	// Result holds the error returned by Execute after it completes.
	Result error
}

// JobListener receives notifications about the lifecycle of job executions. All
// methods are invoked synchronously on the worker goroutine; keep them fast.
type JobListener interface {
	// Name identifies the listener.
	Name() string
	// JobToBeExecuted is called immediately before Execute.
	JobToBeExecuted(ctx context.Context, jec *JobExecutionContext)
	// JobWasExecuted is called after Execute returns, with the resulting
	// error (if any) in jec.Result.
	JobWasExecuted(ctx context.Context, jec *JobExecutionContext)
}

// TriggerListener receives notifications about trigger firing. VetoJobExecution
// allows a listener to prevent a job from running for a given fire.
type TriggerListener interface {
	// Name identifies the listener.
	Name() string
	// TriggerFired is called when a trigger has fired and its job is about
	// to be executed.
	TriggerFired(ctx context.Context, trigger Trigger, jec *JobExecutionContext)
	// VetoJobExecution returns true to prevent the job from executing for
	// this fire.
	VetoJobExecution(ctx context.Context, trigger Trigger, jec *JobExecutionContext) bool
	// TriggerComplete is called after the job has executed (or was vetoed).
	TriggerComplete(ctx context.Context, trigger Trigger, jec *JobExecutionContext)
}

// BaseJobListener is a no-op JobListener that can be embedded to implement only
// the methods of interest.
type BaseJobListener struct{ ListenerName string }

// Name implements JobListener.
func (b BaseJobListener) Name() string { return b.ListenerName }

// JobToBeExecuted implements JobListener.
func (b BaseJobListener) JobToBeExecuted(context.Context, *JobExecutionContext) {}

// JobWasExecuted implements JobListener.
func (b BaseJobListener) JobWasExecuted(context.Context, *JobExecutionContext) {}

// BaseTriggerListener is a no-op TriggerListener that can be embedded to
// implement only the methods of interest.
type BaseTriggerListener struct{ ListenerName string }

// Name implements TriggerListener.
func (b BaseTriggerListener) Name() string { return b.ListenerName }

// TriggerFired implements TriggerListener.
func (b BaseTriggerListener) TriggerFired(context.Context, Trigger, *JobExecutionContext) {}

// VetoJobExecution implements TriggerListener and never vetoes.
func (b BaseTriggerListener) VetoJobExecution(context.Context, Trigger, *JobExecutionContext) bool {
	return false
}

// TriggerComplete implements TriggerListener.
func (b BaseTriggerListener) TriggerComplete(context.Context, Trigger, *JobExecutionContext) {}
