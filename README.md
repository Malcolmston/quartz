# quartz

A Quartz-style job scheduler for Go, written entirely with the standard
library. No cgo, no third-party dependencies.

Quartz provides jobs, triggers (fixed-interval and full cron), a bounded worker
pool, job/trigger listeners, misfire handling, and a pluggable job store. Every
fire-time decision flows through an injectable clock, so schedules are fully
deterministic and testable without sleeping.

## Features

- **Jobs** — any type implementing `Execute(context.Context) error`, plus a
  `JobFunc` adapter and a `JobDataMap` for per-job state.
- **SimpleTrigger** — start time + interval + repeat count (or repeat forever)
  with an optional end time.
- **CronTrigger** — a real cron parser supporting second/minute/hour/
  day-of-month/month/day-of-week with ranges (`1-5`), steps (`*/15`, `10-30/5`),
  lists (`1,3,5`), names (`MON`, `JAN`), and the `*` / `?` wildcards, evaluated
  in any `*time.Location`.
- **Scheduler** — schedule/unschedule, a configurable worker pool, graceful
  shutdown with drain, pause/resume of individual triggers or whole jobs, and
  configurable misfire policies.
- **Listeners** — `JobListener` and `TriggerListener` hooks; trigger listeners
  can veto a job for a given fire.
- **JobStore** — an interface with an in-memory implementation, so a
  database-backed store can be added without touching scheduler logic.
- **Injectable clock** — `Options.Clock` makes cron and next-fire logic
  deterministic in tests.

## Install

```sh
go get github.com/malcolmston/quartz
```

Requires Go 1.24 or newer.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/malcolmston/quartz"
)

func main() {
	// A pool of 4 workers running in real time.
	s := quartz.NewScheduler(quartz.Options{Concurrency: 4})

	job := quartz.NewJobDetail(
		quartz.NewKey("daily-report"),
		quartz.JobFunc(func(ctx context.Context) error {
			fmt.Println("generating report")
			return nil
		}),
	)

	// Fire every day at 10:00:00.
	trigger, err := quartz.NewCronTriggerWithKeys(
		quartz.NewKey("report-trigger"), job.Key(), "0 0 10 * * *")
	if err != nil {
		panic(err)
	}

	if err := s.ScheduleJob(job, trigger); err != nil {
		panic(err)
	}

	if err := s.Start(); err != nil {
		panic(err)
	}
	defer s.Shutdown(true) // graceful drain

	// ... run your application ...
	time.Sleep(time.Second)
}
```

### A repeating interval trigger

```go
trigger := quartz.NewSimpleTrigger(
	quartz.NewKey("heartbeat"), job.Key(),
	time.Now(), 30*time.Second, quartz.RepeatForever)
```

### Deterministic testing with an injected clock

Because triggers compute their next fire time as a pure function of "now", tests
can drive the scheduler with a virtual clock and call `ProcessDue` directly:

```go
now := time.Date(2026, 1, 1, 9, 59, 0, 0, time.UTC)
s := quartz.NewScheduler(quartz.Options{
	Clock: func() time.Time { return now },
})
// ... schedule a 10:00 cron job ...

s.ProcessDue()                                     // nothing due at 09:59
now = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC) // advance the clock
s.ProcessDue()                                     // fires now
```

## Cron syntax

Six fields (a five-field form without seconds is also accepted):

```
 second       0-59
 minute       0-59
 hour         0-23
 day-of-month 1-31
 month        1-12 or JAN-DEC
 day-of-week  0-6  or SUN-SAT (0 and 7 are both Sunday)
```

Day matching follows Unix semantics: when both day-of-month and day-of-week are
restricted, a day matches if *either* field matches; otherwise only the
restricted field applies.

## Development

```sh
go build ./...
go vet ./...
go test -race -cover ./...
```

## License

See repository for license details.
