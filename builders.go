package quartz

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// This file ports the fluent builder API from the original Quartz project:
// JobBuilder, TriggerBuilder and the family of ScheduleBuilder types. They
// provide a readable, chainable way to construct JobDetails and the various
// Trigger implementations without calling their lower-level constructors
// directly.

// builderAutoName is an incrementing counter used to synthesize identities for
// jobs and triggers built without an explicit key, mirroring Quartz's automatic
// name generation.
var builderAutoName uint64

// builderNextAutoName returns a process-unique name with the given prefix.
func builderNextAutoName(prefix string) string {
	n := atomic.AddUint64(&builderAutoName, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// JobBuilder assembles a JobDetail through a fluent, chainable interface. Create
// one with NewJob, configure it, then call Build. The zero value is not usable;
// always start from NewJob.
type JobBuilder struct {
	key     Key
	keySet  bool
	desc    string
	job     Job
	data    JobDataMap
	durable bool
}

// NewJob starts building a JobDetail. Attach the work with OfJob and, if
// desired, an identity with WithIdentity before calling Build.
func NewJob() *JobBuilder {
	return &JobBuilder{data: JobDataMap{}}
}

// OfJob sets the concrete Job implementation the detail will run and returns the
// builder for chaining.
func (b *JobBuilder) OfJob(job Job) *JobBuilder {
	b.job = job
	return b
}

// WithIdentity sets the job's key and returns the builder for chaining.
func (b *JobBuilder) WithIdentity(key Key) *JobBuilder {
	b.key = key
	b.keySet = !key.IsZero()
	return b
}

// WithIdentityName sets the job's key to the given name in the default group and
// returns the builder for chaining.
func (b *JobBuilder) WithIdentityName(name string) *JobBuilder {
	return b.WithIdentity(NewKey(name))
}

// WithIdentityNameGroup sets the job's key to the given name and group and
// returns the builder for chaining.
func (b *JobBuilder) WithIdentityNameGroup(name, group string) *JobBuilder {
	return b.WithIdentity(NewKeyInGroup(name, group))
}

// WithDescription sets the job's human readable description and returns the
// builder for chaining.
func (b *JobBuilder) WithDescription(desc string) *JobBuilder {
	b.desc = desc
	return b
}

// StoreDurably sets whether the job is retained by the store even when no
// triggers reference it and returns the builder for chaining.
func (b *JobBuilder) StoreDurably(durable bool) *JobBuilder {
	b.durable = durable
	return b
}

// UsingJobData adds a single key/value pair to the job's JobDataMap and returns
// the builder for chaining.
func (b *JobBuilder) UsingJobData(key string, value any) *JobBuilder {
	if b.data == nil {
		b.data = JobDataMap{}
	}
	b.data[key] = value
	return b
}

// UsingJobDataMap merges every entry of the given map into the job's data and
// returns the builder for chaining.
func (b *JobBuilder) UsingJobDataMap(data JobDataMap) *JobBuilder {
	if b.data == nil {
		b.data = JobDataMap{}
	}
	for k, v := range data {
		b.data[k] = v
	}
	return b
}

// Build produces the configured JobDetail. It returns an error if no Job was
// supplied. When no identity was set a process-unique name is generated in the
// default group.
func (b *JobBuilder) Build() (*JobDetail, error) {
	if b.job == nil {
		return nil, errors.New("quartz: JobBuilder requires a Job (call OfJob)")
	}
	key := b.key
	if !b.keySet {
		key = NewKey(builderNextAutoName("job"))
	}
	d := NewJobDetail(key, b.job)
	if b.desc != "" {
		d.WithDescription(b.desc)
	}
	if len(b.data) > 0 {
		d.WithData(b.data)
	}
	d.Durable(b.durable)
	return d, nil
}

// ScheduleBuilder is implemented by the schedule builders (simple, cron,
// calendar-interval and daily-time-interval). A TriggerBuilder combines a
// ScheduleBuilder with an identity, job reference and start/end window to
// produce a concrete Trigger.
type ScheduleBuilder interface {
	// BuildTrigger constructs a fully configured Trigger for the given
	// identity and job key. A zero start or end time means the corresponding
	// bound is unset. It returns an error if the schedule is invalid.
	BuildTrigger(key, jobKey Key, start, end time.Time, description string) (Trigger, error)
}

// TriggerBuilder assembles a Trigger by combining an identity, a job reference,
// a firing window and a ScheduleBuilder. Create one with NewTrigger.
type TriggerBuilder struct {
	key      Key
	keySet   bool
	jobKey   Key
	desc     string
	start    time.Time
	startNow bool
	end      time.Time
	schedule ScheduleBuilder
}

// NewTrigger starts building a Trigger. Configure its identity, job and schedule
// then call Build.
func NewTrigger() *TriggerBuilder {
	return &TriggerBuilder{}
}

// WithIdentity sets the trigger's key and returns the builder for chaining.
func (b *TriggerBuilder) WithIdentity(key Key) *TriggerBuilder {
	b.key = key
	b.keySet = !key.IsZero()
	return b
}

// WithIdentityName sets the trigger's key to the given name in the default group
// and returns the builder for chaining.
func (b *TriggerBuilder) WithIdentityName(name string) *TriggerBuilder {
	return b.WithIdentity(NewKey(name))
}

// WithIdentityNameGroup sets the trigger's key to the given name and group and
// returns the builder for chaining.
func (b *TriggerBuilder) WithIdentityNameGroup(name, group string) *TriggerBuilder {
	return b.WithIdentity(NewKeyInGroup(name, group))
}

// ForJob sets the key of the job the trigger will fire and returns the builder
// for chaining.
func (b *TriggerBuilder) ForJob(jobKey Key) *TriggerBuilder {
	b.jobKey = jobKey
	return b
}

// ForJobDetail sets the job reference from an existing JobDetail and returns the
// builder for chaining.
func (b *TriggerBuilder) ForJobDetail(detail *JobDetail) *TriggerBuilder {
	if detail != nil {
		b.jobKey = detail.Key()
	}
	return b
}

// WithDescription sets the trigger's description and returns the builder for
// chaining.
func (b *TriggerBuilder) WithDescription(desc string) *TriggerBuilder {
	b.desc = desc
	return b
}

// StartAt sets the earliest time the trigger may fire and returns the builder
// for chaining.
func (b *TriggerBuilder) StartAt(t time.Time) *TriggerBuilder {
	b.start = t
	b.startNow = false
	return b
}

// StartNow marks the trigger to begin at the moment Build is called and returns
// the builder for chaining.
func (b *TriggerBuilder) StartNow() *TriggerBuilder {
	b.startNow = true
	b.start = time.Time{}
	return b
}

// EndAt sets the latest time the trigger may fire and returns the builder for
// chaining. The zero time means no end.
func (b *TriggerBuilder) EndAt(t time.Time) *TriggerBuilder {
	b.end = t
	return b
}

// WithSchedule sets the ScheduleBuilder that determines the trigger's firing
// cadence and returns the builder for chaining.
func (b *TriggerBuilder) WithSchedule(s ScheduleBuilder) *TriggerBuilder {
	b.schedule = s
	return b
}

// Build produces the configured Trigger. It requires a job reference (set with
// ForJob or ForJobDetail). When no identity was set a process-unique name is
// generated. When no schedule was set a fire-once SimpleSchedule is used. When
// no start time was set the trigger starts at the current time, matching the
// original Quartz default.
func (b *TriggerBuilder) Build() (Trigger, error) {
	if b.jobKey.IsZero() {
		return nil, errors.New("quartz: TriggerBuilder requires a job (call ForJob)")
	}
	key := b.key
	if !b.keySet {
		key = NewKey(builderNextAutoName("trigger"))
	}
	start := b.start
	if b.startNow || start.IsZero() {
		start = time.Now()
	}
	sched := b.schedule
	if sched == nil {
		sched = SimpleSchedule()
	}
	return sched.BuildTrigger(key, b.jobKey, start, b.end, b.desc)
}

// SimpleScheduleBuilder builds a SimpleTrigger: a start time plus a fixed
// interval repeated a bounded number of times or forever. Create one with
// SimpleSchedule.
type SimpleScheduleBuilder struct {
	interval    time.Duration
	repeatCount int
	policy      MisfirePolicy
}

// SimpleSchedule starts building a simple interval schedule. Without further
// configuration it produces a trigger that fires exactly once at the start
// time.
func SimpleSchedule() *SimpleScheduleBuilder {
	return &SimpleScheduleBuilder{repeatCount: 0}
}

// WithInterval sets the repeat interval and returns the builder for chaining.
func (b *SimpleScheduleBuilder) WithInterval(d time.Duration) *SimpleScheduleBuilder {
	b.interval = d
	return b
}

// WithIntervalInSeconds sets the repeat interval in seconds and returns the
// builder for chaining.
func (b *SimpleScheduleBuilder) WithIntervalInSeconds(seconds int) *SimpleScheduleBuilder {
	b.interval = time.Duration(seconds) * time.Second
	return b
}

// WithIntervalInMinutes sets the repeat interval in minutes and returns the
// builder for chaining.
func (b *SimpleScheduleBuilder) WithIntervalInMinutes(minutes int) *SimpleScheduleBuilder {
	b.interval = time.Duration(minutes) * time.Minute
	return b
}

// WithIntervalInHours sets the repeat interval in hours and returns the builder
// for chaining.
func (b *SimpleScheduleBuilder) WithIntervalInHours(hours int) *SimpleScheduleBuilder {
	b.interval = time.Duration(hours) * time.Hour
	return b
}

// WithRepeatCount sets the number of repeats after the initial fire and returns
// the builder for chaining. A count of N yields N+1 total fires.
func (b *SimpleScheduleBuilder) WithRepeatCount(count int) *SimpleScheduleBuilder {
	b.repeatCount = count
	return b
}

// RepeatForever configures unbounded repeats and returns the builder for
// chaining.
func (b *SimpleScheduleBuilder) RepeatForever() *SimpleScheduleBuilder {
	b.repeatCount = RepeatForever
	return b
}

// WithMisfirePolicy sets the misfire policy and returns the builder for
// chaining.
func (b *SimpleScheduleBuilder) WithMisfirePolicy(p MisfirePolicy) *SimpleScheduleBuilder {
	b.policy = p
	return b
}

// BuildTrigger implements ScheduleBuilder.
func (b *SimpleScheduleBuilder) BuildTrigger(key, jobKey Key, start, end time.Time, description string) (Trigger, error) {
	t := NewSimpleTrigger(key, jobKey, start, b.interval, b.repeatCount)
	if !end.IsZero() {
		t.WithEndTime(end)
	}
	if description != "" {
		t.WithDescription(description)
	}
	t.WithMisfirePolicy(b.policy)
	return t, nil
}

// CronScheduleBuilder builds a CronTrigger from a cron expression evaluated in a
// chosen time zone. Create one with CronSchedule or a convenience constructor.
type CronScheduleBuilder struct {
	expr   string
	loc    *time.Location
	policy MisfirePolicy
}

// CronSchedule starts building a cron schedule from the given expression. The
// expression is validated immediately and an error is returned if it is
// invalid.
func CronSchedule(expr string) (*CronScheduleBuilder, error) {
	if _, err := ParseCron(expr); err != nil {
		return nil, err
	}
	return &CronScheduleBuilder{expr: expr, loc: time.UTC}, nil
}

// CronScheduleDailyAt builds a cron schedule that fires every day at the given
// hour (0-23) and minute (0-59).
func CronScheduleDailyAt(hour, minute int) *CronScheduleBuilder {
	return &CronScheduleBuilder{
		expr: fmt.Sprintf("0 %d %d ? * *", minute, hour),
		loc:  time.UTC,
	}
}

// CronScheduleWeeklyOn builds a cron schedule that fires every week on the given
// weekday at the given hour and minute.
func CronScheduleWeeklyOn(day time.Weekday, hour, minute int) *CronScheduleBuilder {
	return &CronScheduleBuilder{
		expr: fmt.Sprintf("0 %d %d ? * %d", minute, hour, int(day)),
		loc:  time.UTC,
	}
}

// CronScheduleMonthlyOn builds a cron schedule that fires every month on the
// given day-of-month at the given hour and minute.
func CronScheduleMonthlyOn(dayOfMonth, hour, minute int) *CronScheduleBuilder {
	return &CronScheduleBuilder{
		expr: fmt.Sprintf("0 %d %d %d * ?", minute, hour, dayOfMonth),
		loc:  time.UTC,
	}
}

// In sets the time zone used to evaluate the cron expression and returns the
// builder for chaining. A nil location is ignored.
func (b *CronScheduleBuilder) In(loc *time.Location) *CronScheduleBuilder {
	if loc != nil {
		b.loc = loc
	}
	return b
}

// WithMisfirePolicy sets the misfire policy and returns the builder for
// chaining.
func (b *CronScheduleBuilder) WithMisfirePolicy(p MisfirePolicy) *CronScheduleBuilder {
	b.policy = p
	return b
}

// BuildTrigger implements ScheduleBuilder.
func (b *CronScheduleBuilder) BuildTrigger(key, jobKey Key, start, end time.Time, description string) (Trigger, error) {
	t, err := NewCronTriggerWithKeys(key, jobKey, b.expr)
	if err != nil {
		return nil, err
	}
	t.In(b.loc)
	if !start.IsZero() {
		t.StartingAt(start)
	}
	if !end.IsZero() {
		t.EndingAt(end)
	}
	if description != "" {
		t.WithDescription(description)
	}
	t.WithMisfirePolicy(b.policy)
	return t, nil
}

// CalendarIntervalScheduleBuilder builds a CalendarIntervalTrigger that advances
// by a whole calendar unit (day, week, month, year, or a sub-day duration).
// Create one with CalendarIntervalSchedule.
type CalendarIntervalScheduleBuilder struct {
	count  int
	unit   IntervalUnit
	loc    *time.Location
	policy MisfirePolicy
}

// CalendarIntervalSchedule starts building a calendar-interval schedule. The
// default is one day.
func CalendarIntervalSchedule() *CalendarIntervalScheduleBuilder {
	return &CalendarIntervalScheduleBuilder{count: 1, unit: IntervalDay, loc: time.UTC}
}

// WithInterval sets the interval count and unit and returns the builder for
// chaining.
func (b *CalendarIntervalScheduleBuilder) WithInterval(count int, unit IntervalUnit) *CalendarIntervalScheduleBuilder {
	b.count = count
	b.unit = unit
	return b
}

// WithIntervalInDays sets a day interval and returns the builder for chaining.
func (b *CalendarIntervalScheduleBuilder) WithIntervalInDays(days int) *CalendarIntervalScheduleBuilder {
	b.count = days
	b.unit = IntervalDay
	return b
}

// WithIntervalInWeeks sets a week interval and returns the builder for chaining.
func (b *CalendarIntervalScheduleBuilder) WithIntervalInWeeks(weeks int) *CalendarIntervalScheduleBuilder {
	b.count = weeks
	b.unit = IntervalWeek
	return b
}

// WithIntervalInMonths sets a month interval and returns the builder for
// chaining.
func (b *CalendarIntervalScheduleBuilder) WithIntervalInMonths(months int) *CalendarIntervalScheduleBuilder {
	b.count = months
	b.unit = IntervalMonth
	return b
}

// WithIntervalInYears sets a year interval and returns the builder for chaining.
func (b *CalendarIntervalScheduleBuilder) WithIntervalInYears(years int) *CalendarIntervalScheduleBuilder {
	b.count = years
	b.unit = IntervalYear
	return b
}

// In sets the time zone used to evaluate calendar advancement and returns the
// builder for chaining. A nil location is ignored.
func (b *CalendarIntervalScheduleBuilder) In(loc *time.Location) *CalendarIntervalScheduleBuilder {
	if loc != nil {
		b.loc = loc
	}
	return b
}

// WithMisfirePolicy sets the misfire policy and returns the builder for
// chaining.
func (b *CalendarIntervalScheduleBuilder) WithMisfirePolicy(p MisfirePolicy) *CalendarIntervalScheduleBuilder {
	b.policy = p
	return b
}

// BuildTrigger implements ScheduleBuilder.
func (b *CalendarIntervalScheduleBuilder) BuildTrigger(key, jobKey Key, start, end time.Time, description string) (Trigger, error) {
	t := NewCalendarIntervalTrigger(key, jobKey, start, b.unit, b.count)
	t.In(b.loc)
	if !end.IsZero() {
		t.WithEndTime(end)
	}
	if description != "" {
		t.WithDescription(description)
	}
	t.WithMisfirePolicy(b.policy)
	return t, nil
}

// DailyTimeIntervalScheduleBuilder builds a DailyTimeIntervalTrigger that fires
// repeatedly within a daily time window on selected days of the week. Create one
// with DailyTimeIntervalSchedule.
type DailyTimeIntervalScheduleBuilder struct {
	count       int
	unit        IntervalUnit
	startTOD    TimeOfDay
	endTOD      TimeOfDay
	days        []time.Weekday
	repeatCount int
	repeatSet   bool
	loc         *time.Location
	policy      MisfirePolicy
}

// DailyTimeIntervalSchedule starts building a daily-time-interval schedule. The
// default is a one-minute interval spanning the whole day, every day.
func DailyTimeIntervalSchedule() *DailyTimeIntervalScheduleBuilder {
	return &DailyTimeIntervalScheduleBuilder{
		count:    1,
		unit:     IntervalMinute,
		startTOD: NewTimeOfDay(0, 0, 0),
		endTOD:   NewTimeOfDay(23, 59, 59),
		loc:      time.UTC,
	}
}

// WithInterval sets the interval count and (sub-day) unit and returns the
// builder for chaining.
func (b *DailyTimeIntervalScheduleBuilder) WithInterval(count int, unit IntervalUnit) *DailyTimeIntervalScheduleBuilder {
	b.count = count
	b.unit = unit
	return b
}

// WithIntervalInSeconds sets a seconds interval and returns the builder for
// chaining.
func (b *DailyTimeIntervalScheduleBuilder) WithIntervalInSeconds(seconds int) *DailyTimeIntervalScheduleBuilder {
	b.count = seconds
	b.unit = IntervalSecond
	return b
}

// WithIntervalInMinutes sets a minutes interval and returns the builder for
// chaining.
func (b *DailyTimeIntervalScheduleBuilder) WithIntervalInMinutes(minutes int) *DailyTimeIntervalScheduleBuilder {
	b.count = minutes
	b.unit = IntervalMinute
	return b
}

// WithIntervalInHours sets an hours interval and returns the builder for
// chaining.
func (b *DailyTimeIntervalScheduleBuilder) WithIntervalInHours(hours int) *DailyTimeIntervalScheduleBuilder {
	b.count = hours
	b.unit = IntervalHour
	return b
}

// StartingDailyAt sets the earliest time-of-day the trigger may fire and returns
// the builder for chaining.
func (b *DailyTimeIntervalScheduleBuilder) StartingDailyAt(tod TimeOfDay) *DailyTimeIntervalScheduleBuilder {
	b.startTOD = tod
	return b
}

// EndingDailyAt sets the latest time-of-day the trigger may fire and returns the
// builder for chaining.
func (b *DailyTimeIntervalScheduleBuilder) EndingDailyAt(tod TimeOfDay) *DailyTimeIntervalScheduleBuilder {
	b.endTOD = tod
	return b
}

// OnDaysOfWeek restricts firing to the given weekdays and returns the builder
// for chaining. Passing no days leaves the current selection unchanged.
func (b *DailyTimeIntervalScheduleBuilder) OnDaysOfWeek(days ...time.Weekday) *DailyTimeIntervalScheduleBuilder {
	if len(days) > 0 {
		b.days = append([]time.Weekday(nil), days...)
	}
	return b
}

// OnEveryDay allows firing on all seven days of the week and returns the builder
// for chaining.
func (b *DailyTimeIntervalScheduleBuilder) OnEveryDay() *DailyTimeIntervalScheduleBuilder {
	b.days = []time.Weekday{
		time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday,
	}
	return b
}

// OnMondayThroughFriday restricts firing to weekdays and returns the builder for
// chaining.
func (b *DailyTimeIntervalScheduleBuilder) OnMondayThroughFriday() *DailyTimeIntervalScheduleBuilder {
	b.days = []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday,
	}
	return b
}

// OnSaturdayAndSunday restricts firing to the weekend and returns the builder
// for chaining.
func (b *DailyTimeIntervalScheduleBuilder) OnSaturdayAndSunday() *DailyTimeIntervalScheduleBuilder {
	b.days = []time.Weekday{time.Saturday, time.Sunday}
	return b
}

// WithRepeatCount caps the number of fires per day and returns the builder for
// chaining. Without it the trigger repeats for the whole window.
func (b *DailyTimeIntervalScheduleBuilder) WithRepeatCount(count int) *DailyTimeIntervalScheduleBuilder {
	b.repeatCount = count
	b.repeatSet = true
	return b
}

// In sets the time zone used to evaluate the daily window and returns the
// builder for chaining. A nil location is ignored.
func (b *DailyTimeIntervalScheduleBuilder) In(loc *time.Location) *DailyTimeIntervalScheduleBuilder {
	if loc != nil {
		b.loc = loc
	}
	return b
}

// WithMisfirePolicy sets the misfire policy and returns the builder for
// chaining.
func (b *DailyTimeIntervalScheduleBuilder) WithMisfirePolicy(p MisfirePolicy) *DailyTimeIntervalScheduleBuilder {
	b.policy = p
	return b
}

// BuildTrigger implements ScheduleBuilder.
func (b *DailyTimeIntervalScheduleBuilder) BuildTrigger(key, jobKey Key, start, end time.Time, description string) (Trigger, error) {
	t := NewDailyTimeIntervalTrigger(key, jobKey, b.startTOD, b.endTOD, b.unit, b.count)
	t.In(b.loc)
	if len(b.days) > 0 {
		t.OnDaysOfWeek(b.days...)
	}
	if !start.IsZero() {
		t.StartingAt(start)
	}
	if !end.IsZero() {
		t.EndingAt(end)
	}
	if b.repeatSet {
		t.WithRepeatCount(b.repeatCount)
	}
	if description != "" {
		t.WithDescription(description)
	}
	t.WithMisfirePolicy(b.policy)
	return t, nil
}
