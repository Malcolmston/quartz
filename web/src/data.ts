// Library content for the quartz documentation site. Mirrors the shape used by
// the malcolmston/go landing site's data.ts so the sibling sites stay in sync.
export interface Lib {
  id: string; name: string; icon: string; accent: string; pkg: string; node: string;
  repo: string; docs: string; tagline: string; blurb: string; tags: string[];
  features: string[]; node_code: string; go_code: string; integrate: string;
}

export const NODE_ACCENT = '#8cc84b';

export const QUARTZ: Lib = {
  id:"quartz", name:"quartz", icon:'<i class="fa-solid fa-clock"></i>', accent:"#8b6df0",
  pkg:"github.com/malcolmston/quartz", node:"quartz-scheduler/quartz",
  repo:"https://github.com/malcolmston/quartz", docs:"https://malcolmston.github.io/quartz/",
  tagline:"A Quartz-style job scheduler for Go, stdlib only.",
  blurb:"A from-scratch, standard-library-only Go take on the classic Quartz job scheduler — no cgo, no "+
    "third-party dependencies. You get Jobs (any Execute(context.Context) error, plus a JobFunc adapter "+
    "and a JobDataMap for per-job state), two Trigger implementations, and a Scheduler that fires due "+
    "triggers on a bounded worker pool. SimpleTrigger covers start-time-plus-interval-plus-repeat "+
    "schedules; CronTrigger runs on a real cron parser supporting five- or six-field expressions with "+
    "ranges, steps, lists, names and the * / ? wildcards, evaluated in any *time.Location. The scheduler "+
    "adds graceful shutdown with drain, per-trigger and per-job pause/resume, configurable misfire "+
    "policies, and job/trigger listeners that can veto a fire. Storage sits behind a JobStore interface "+
    "with an in-memory implementation, and every fire-time decision flows through an injectable clock so "+
    "schedules are fully deterministic and testable without sleeping.",
  tags:["Job","JobDetail","SimpleTrigger","CronTrigger","ParseCron","Scheduler","worker pool","misfire","listeners","JobStore","injectable clock","stdlib-only"],
  features:[
    "Jobs — any <code>Job</code> (<code>Execute(context.Context) error</code>), a <code>JobFunc</code> adapter, and a <code>JobDataMap</code> of per-job state carried through each fire",
    "<code>JobDetail</code> — names a job via a <code>Key</code>, with <code>WithDescription</code>, <code>WithData</code> and <code>Durable</code> builder options",
    "<code>SimpleTrigger</code> — start time plus a fixed <code>Interval</code> and a repeat count (or <code>RepeatForever</code>), with an optional end time",
    "<code>CronTrigger</code> over a real parser — <code>ParseCron</code>/<code>CronExpression</code> handle 5/6 fields, ranges (<code>1-5</code>), steps (<code>*/15</code>), lists, names and <code>?</code>, evaluated in any <code>*time.Location</code> via <code>In</code>",
    "<code>Scheduler</code> — <code>NewScheduler</code> with a bounded <code>Options.Concurrency</code> worker pool, <code>ScheduleJob</code>, <code>Start</code>, and <code>Shutdown</code> with optional drain",
    "Pause / resume — <code>PauseTrigger</code>/<code>ResumeTrigger</code> and <code>PauseJob</code>/<code>ResumeJob</code>, plus out-of-band <code>TriggerJob</code>",
    "Misfire policies — <code>MisfireSmart</code>, <code>MisfireFireNow</code>, <code>MisfireIgnore</code> and <code>MisfireDoNothing</code> via <code>WithMisfirePolicy</code>",
    "Listeners — <code>JobListener</code> and <code>TriggerListener</code> hooks (embed <code>BaseJobListener</code>/<code>BaseTriggerListener</code>); a trigger listener can <code>VetoJobExecution</code>",
    "Pluggable storage — a <code>JobStore</code> interface with an in-memory <code>MemoryJobStore</code> (<code>NewMemoryJobStore</code>)",
    "Injectable clock — <code>Options.Clock</code> plus <code>ProcessDue</code> drive cron and next-fire logic deterministically in tests, without sleeping",
    "Zero dependencies — pure Go standard library, nothing to audit but the toolchain"
  ],
  node_code:
`// Java — quartz-scheduler/quartz
JobDetail job = JobBuilder.newJob(ReportJob.class)
    .withIdentity("daily-report")
    .build();

Trigger trigger = TriggerBuilder.newTrigger()
    .withIdentity("report-trigger")
    .withSchedule(CronScheduleBuilder.cronSchedule("0 0 10 * * ?"))
    .build();

Scheduler scheduler = StdSchedulerFactory.getDefaultScheduler();
scheduler.start();
scheduler.scheduleJob(job, trigger);`,
  go_code:
`import "github.com/malcolmston/quartz"

// A pool of 4 workers running in real time.
s := quartz.NewScheduler(quartz.Options{Concurrency: 4})

job := quartz.NewJobDetail(quartz.NewKey("daily-report"),
    quartz.JobFunc(func(ctx context.Context) error {
        fmt.Println("generating report")
        return nil
    }))

// Fire every day at 10:00:00.
trigger, _ := quartz.NewCronTriggerWithKeys(
    quartz.NewKey("report-trigger"), job.Key(), "0 0 10 * * *")

s.ScheduleJob(job, trigger)
s.Start()
defer s.Shutdown(true) // graceful drain`,
  integrate:
`<span class="tok-c">// A repeating interval trigger: fire every 30s, forever.</span>
job := quartz.NewJobDetail(quartz.NewKey("heartbeat"),
    quartz.JobFunc(func(ctx context.Context) error { return nil }))
beat := quartz.NewSimpleTrigger(quartz.NewKey("beat"), job.Key(),
    time.Now(), 30*time.Second, quartz.RepeatForever)
_ = s.ScheduleJob(job, beat)

<span class="tok-c">// A cron trigger in a chosen time zone, skipping missed fires.</span>
ny, _ := time.LoadLocation("America/New_York")
nightly, _ := quartz.NewCronTriggerWithKeys(
    quartz.NewKey("nightly"), job.Key(), "0 0 2 * * *")
nightly.In(ny).WithMisfirePolicy(quartz.MisfireDoNothing)

<span class="tok-c">// Pause and later resume a single trigger; misfires are reconciled.</span>
_ = s.PauseTrigger(beat.Key())
_ = s.ResumeTrigger(beat.Key())

<span class="tok-c">// Deterministic testing: inject a clock and call ProcessDue directly.</span>
now := time.Date(2026, 1, 1, 9, 59, 0, 0, time.UTC)
t := quartz.NewScheduler(quartz.Options{Clock: func() time.Time { return now }})
_ = t.ProcessDue()                                     // nothing due at 09:59
now = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)     // advance the clock
_ = t.ProcessDue()                                     // fires now`
};
