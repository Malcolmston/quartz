package quartz

import (
	"context"
	"testing"
	"time"
)

func builderNoopJob() Job {
	return JobFunc(func(context.Context) error { return nil })
}

func TestJobBuilderBuild(t *testing.T) {
	job := builderNoopJob()
	d, err := NewJob().
		OfJob(job).
		WithIdentityNameGroup("report", "reports").
		WithDescription("nightly report").
		StoreDurably(true).
		UsingJobData("attempts", 3).
		UsingJobDataMap(JobDataMap{"owner": "ops"}).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := d.Key(); got != NewKeyInGroup("report", "reports") {
		t.Errorf("key = %v", got)
	}
	if d.Description() != "nightly report" {
		t.Errorf("desc = %q", d.Description())
	}
	if !d.IsDurable() {
		t.Errorf("durable = false, want true")
	}
	if n, ok := d.Data().GetInt("attempts"); !ok || n != 3 {
		t.Errorf("attempts = %d,%v", n, ok)
	}
	if s, ok := d.Data().GetString("owner"); !ok || s != "ops" {
		t.Errorf("owner = %q,%v", s, ok)
	}
}

func TestJobBuilderRequiresJob(t *testing.T) {
	if _, err := NewJob().WithIdentityName("x").Build(); err == nil {
		t.Fatal("expected error when no job supplied")
	}
}

func TestJobBuilderAutoName(t *testing.T) {
	d, err := NewJob().OfJob(builderNoopJob()).Build()
	if err != nil {
		t.Fatal(err)
	}
	if d.Key().IsZero() {
		t.Fatal("expected generated key")
	}
	if d.Key().Group != DefaultGroup {
		t.Errorf("group = %q", d.Key().Group)
	}
}

func TestTriggerBuilderRequiresJob(t *testing.T) {
	if _, err := NewTrigger().WithSchedule(SimpleSchedule()).Build(); err == nil {
		t.Fatal("expected error when no job supplied")
	}
}

func TestTriggerBuilderSimpleSchedule(t *testing.T) {
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	tr, err := NewTrigger().
		WithIdentityName("beat").
		ForJob(NewKey("job")).
		StartAt(start).
		WithSchedule(SimpleSchedule().
			WithIntervalInMinutes(15).
			WithRepeatCount(4)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	st, ok := tr.(*SimpleTrigger)
	if !ok {
		t.Fatalf("type = %T, want *SimpleTrigger", tr)
	}
	if st.Interval() != 15*time.Minute {
		t.Errorf("interval = %s", st.Interval())
	}
	if st.RepeatCount() != 4 {
		t.Errorf("repeat = %d", st.RepeatCount())
	}
	if got := st.ComputeFirstFireTime(start); !got.Equal(start) {
		t.Errorf("first fire = %v, want %v", got, start)
	}
	want := start.Add(15 * time.Minute)
	if got := st.Triggered(start); !got.Equal(want) {
		t.Errorf("second fire = %v, want %v", got, want)
	}
}

func TestSimpleScheduleRepeatForever(t *testing.T) {
	s := SimpleSchedule().WithInterval(time.Second).RepeatForever()
	tr, err := s.BuildTrigger(NewKey("t"), NewKey("j"), time.Unix(0, 0).UTC(), time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if tr.(*SimpleTrigger).RepeatCount() != RepeatForever {
		t.Errorf("repeat = %d", tr.(*SimpleTrigger).RepeatCount())
	}
}

func TestTriggerBuilderDefaultFireOnce(t *testing.T) {
	start := time.Date(2026, 5, 5, 5, 5, 5, 0, time.UTC)
	tr, err := NewTrigger().ForJob(NewKey("j")).StartAt(start).Build()
	if err != nil {
		t.Fatal(err)
	}
	st := tr.(*SimpleTrigger)
	if got := st.ComputeFirstFireTime(start); !got.Equal(start) {
		t.Errorf("first = %v", got)
	}
	if got := st.Triggered(start); !got.IsZero() {
		t.Errorf("fire-once trigger fired again at %v", got)
	}
}

func TestCronScheduleDailyAt(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr, err := NewTrigger().
		WithIdentityName("daily").
		ForJob(NewKey("j")).
		StartAt(start).
		WithSchedule(CronScheduleDailyAt(10, 30)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ct := tr.(*CronTrigger)
	want := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	if got := ct.ComputeFirstFireTime(start); !got.Equal(want) {
		t.Errorf("first fire = %v, want %v", got, want)
	}
	next := time.Date(2026, 1, 2, 10, 30, 0, 0, time.UTC)
	if got := ct.Triggered(want); !got.Equal(next) {
		t.Errorf("second fire = %v, want %v", got, next)
	}
}

func TestCronScheduleWeeklyOn(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr, err := CronScheduleWeeklyOn(time.Wednesday, 8, 0).
		BuildTrigger(NewKey("t"), NewKey("j"), start, time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	got := tr.(*CronTrigger).ComputeFirstFireTime(start)
	if got.Weekday() != time.Wednesday || got.Hour() != 8 || got.Minute() != 0 {
		t.Errorf("first fire = %v (weekday %v)", got, got.Weekday())
	}
	if !got.After(start) {
		t.Errorf("fire %v not after start %v", got, start)
	}
}

func TestCronScheduleMonthlyOn(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr, err := CronScheduleMonthlyOn(15, 12, 0).
		BuildTrigger(NewKey("t"), NewKey("j"), start, time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	if got := tr.(*CronTrigger).ComputeFirstFireTime(start); !got.Equal(want) {
		t.Errorf("first fire = %v, want %v", got, want)
	}
}

func TestCronScheduleInvalid(t *testing.T) {
	if _, err := CronSchedule("not a cron"); err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestCalendarIntervalSchedule(t *testing.T) {
	start := time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)
	tr, err := NewTrigger().
		WithIdentityName("cal").
		ForJob(NewKey("j")).
		StartAt(start).
		WithSchedule(CalendarIntervalSchedule().WithIntervalInDays(2)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	cit := tr.(*CalendarIntervalTrigger)
	if cit.Unit() != IntervalDay || cit.Count() != 2 {
		t.Errorf("unit/count = %v/%d", cit.Unit(), cit.Count())
	}
	if got := cit.ComputeFirstFireTime(start); !got.Equal(start) {
		t.Errorf("first = %v, want %v", got, start)
	}
	want := time.Date(2026, 3, 3, 6, 0, 0, 0, time.UTC)
	if got := cit.Triggered(start); !got.Equal(want) {
		t.Errorf("second = %v, want %v", got, want)
	}
}

func TestCalendarIntervalScheduleMonths(t *testing.T) {
	b := CalendarIntervalSchedule().WithIntervalInMonths(3)
	tr, _ := b.BuildTrigger(NewKey("t"), NewKey("j"),
		time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), time.Time{}, "")
	cit := tr.(*CalendarIntervalTrigger)
	if cit.Unit() != IntervalMonth || cit.Count() != 3 {
		t.Errorf("unit/count = %v/%d", cit.Unit(), cit.Count())
	}
}

func TestDailyTimeIntervalSchedule(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr, err := NewTrigger().
		WithIdentityName("dti").
		ForJob(NewKey("j")).
		StartAt(start).
		WithSchedule(DailyTimeIntervalSchedule().
			WithIntervalInMinutes(30).
			StartingDailyAt(NewTimeOfDay(9, 0, 0)).
			EndingDailyAt(NewTimeOfDay(17, 0, 0)).
			OnMondayThroughFriday()).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	dti := tr.(*DailyTimeIntervalTrigger)
	if dti.Unit() != IntervalMinute || dti.Count() != 30 {
		t.Errorf("unit/count = %v/%d", dti.Unit(), dti.Count())
	}
	if len(dti.DaysOfWeek()) != 5 {
		t.Errorf("days = %v", dti.DaysOfWeek())
	}
	// 2026-01-01 is a Thursday, within Mon-Fri, so first slot is 09:00.
	want := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	if got := dti.ComputeFirstFireTime(start); !got.Equal(want) {
		t.Errorf("first = %v, want %v", got, want)
	}
	next := time.Date(2026, 1, 1, 9, 30, 0, 0, time.UTC)
	if got := dti.Triggered(want); !got.Equal(next) {
		t.Errorf("second = %v, want %v", got, next)
	}
}

func TestDailyTimeIntervalWeekendSelection(t *testing.T) {
	b := DailyTimeIntervalSchedule().OnSaturdayAndSunday()
	tr, _ := b.BuildTrigger(NewKey("t"), NewKey("j"),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{}, "")
	days := tr.(*DailyTimeIntervalTrigger).DaysOfWeek()
	if len(days) != 2 {
		t.Fatalf("days = %v", days)
	}
}

func TestScheduleBuilderMisfirePolicy(t *testing.T) {
	tr, _ := SimpleSchedule().
		WithMisfirePolicy(MisfireFireNow).
		BuildTrigger(NewKey("t"), NewKey("j"), time.Unix(0, 0), time.Time{}, "")
	if tr.MisfirePolicy() != MisfireFireNow {
		t.Errorf("policy = %v", tr.MisfirePolicy())
	}
}

func TestTriggerBuilderStartNow(t *testing.T) {
	before := time.Now()
	tr, err := NewTrigger().ForJob(NewKey("j")).StartNow().Build()
	if err != nil {
		t.Fatal(err)
	}
	got := tr.(*SimpleTrigger).StartTime()
	if got.Before(before.Add(-time.Second)) {
		t.Errorf("start %v is before build time %v", got, before)
	}
}

func TestBuildEndToEndWithScheduler(t *testing.T) {
	now := time.Date(2026, 1, 1, 9, 59, 0, 0, time.UTC)
	s := NewScheduler(Options{Clock: func() time.Time { return now }})
	job, err := NewJob().OfJob(builderNoopJob()).WithIdentityName("j").Build()
	if err != nil {
		t.Fatal(err)
	}
	trig, err := NewTrigger().
		WithIdentityName("t").
		ForJobDetail(job).
		StartAt(now).
		WithSchedule(CronScheduleDailyAt(10, 0)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ScheduleJob(job, trig); err != nil {
		t.Fatalf("ScheduleJob: %v", err)
	}
	stored, err := s.Store().GetTrigger(NewKey("t"))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	if got := stored.Trigger.NextFireTime(); !got.Equal(want) {
		t.Errorf("next fire = %v, want %v", got, want)
	}
}

func BenchmarkTriggerBuilderBuild(b *testing.B) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = NewTrigger().
			WithIdentityName("t").
			ForJob(NewKey("j")).
			StartAt(start).
			WithSchedule(CronScheduleDailyAt(10, 30)).
			Build()
	}
}
