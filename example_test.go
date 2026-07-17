package quartz_test

import (
	"context"
	"fmt"
	"time"

	"github.com/malcolmston/quartz"
)

// Example demonstrates scheduling a cron job and firing it deterministically
// with an injected clock. Because the clock is controlled by the test, the
// scheduler can be driven with ProcessDue instead of sleeping.
func Example() {
	// A fixed clock lets us fire triggers without real time passing.
	now := time.Date(2026, 1, 1, 9, 59, 0, 0, time.UTC)
	s := quartz.NewScheduler(quartz.Options{
		Clock: func() time.Time { return now },
	})

	job := quartz.NewJobDetail(
		quartz.NewKey("daily-report"),
		quartz.JobFunc(func(ctx context.Context) error {
			fmt.Println("report generated")
			return nil
		}),
	)

	// Fire at 10:00:00 every day.
	trigger, err := quartz.NewCronTriggerWithKeys(
		quartz.NewKey("report-trigger"), job.Key(), "0 0 10 * * *")
	if err != nil {
		panic(err)
	}

	if err := s.ScheduleJob(job, trigger); err != nil {
		panic(err)
	}

	fmt.Println("next fire:", trigger.NextFireTime().Format("2006-01-02 15:04:05"))

	// Nothing is due yet at 09:59.
	fmt.Println("fired at 09:59:", s.ProcessDue())

	// Advance to 10:00 and process again.
	now = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	fmt.Println("fired at 10:00:", s.ProcessDue())

	// Output:
	// next fire: 2026-01-01 10:00:00
	// fired at 09:59: 0
	// report generated
	// fired at 10:00: 1
}

// ExampleSimpleTrigger shows a bounded, repeating interval trigger and its fire
// times computed with an injected clock.
func ExampleSimpleTrigger() {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trig := quartz.NewSimpleTrigger(
		quartz.NewKey("t"), quartz.NewKey("j"),
		start, 30*time.Minute, 2) // 3 total fires

	trig.ComputeFirstFireTime(start)
	for trig.WillFireAgain() {
		fmt.Println(trig.NextFireTime().Format("15:04"))
		trig.Triggered(trig.NextFireTime())
	}

	// Output:
	// 00:00
	// 00:30
	// 01:00
}
