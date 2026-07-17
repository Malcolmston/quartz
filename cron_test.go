package quartz

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return tm.UTC()
}

func TestParseCronErrors(t *testing.T) {
	cases := []string{
		"",              // too few fields
		"* * *",         // 3 fields
		"* * * * * * *", // 7 fields
		"60 * * * * *",  // second out of range
		"* 60 * * * *",  // minute out of range
		"* * 24 * * *",  // hour out of range
		"* * * 0 * *",   // day-of-month below min
		"* * * 32 * *",  // day-of-month above max
		"* * * * 13 *",  // month out of range
		"* * * * FOO *", // bad month name
		"* * * * * 9",   // dow out of range
		"*/0 * * * * *", // zero step
		"*/x * * * * *", // non-numeric step
		"5-1 * * * * *", // inverted range
		"abc * * * * *", // non-numeric value
	}
	for _, expr := range cases {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) expected error, got nil", expr)
		}
	}
}

func TestParseCronFiveFields(t *testing.T) {
	// Five-field expression assumes seconds=0.
	ce := MustParseCron("*/15 * * * *")
	got := ce.Next(mustTime(t, "2026-01-01 10:02:30"))
	want := mustTime(t, "2026-01-01 10:15:00")
	if !got.Equal(want) {
		t.Fatalf("Next = %v, want %v", got, want)
	}
}

func TestCronNextEverySecond(t *testing.T) {
	ce := MustParseCron("* * * * * *")
	got := ce.Next(mustTime(t, "2026-01-01 00:00:00"))
	want := mustTime(t, "2026-01-01 00:00:01")
	if !got.Equal(want) {
		t.Fatalf("Next = %v, want %v", got, want)
	}
}

func TestCronNextCases(t *testing.T) {
	cases := []struct {
		name string
		expr string
		from string
		want string
	}{
		{"step-minutes", "0 */15 * * * *", "2026-01-01 10:07:00", "2026-01-01 10:15:00"},
		{"list-minutes", "0 0,30 * * * *", "2026-01-01 10:05:00", "2026-01-01 10:30:00"},
		{"range-hours", "0 0 9-17 * * *", "2026-01-01 08:00:00", "2026-01-01 09:00:00"},
		{"range-hours-wrap", "0 0 9-17 * * *", "2026-01-01 18:00:00", "2026-01-02 09:00:00"},
		{"daily-midnight", "0 0 0 * * *", "2026-01-01 12:00:00", "2026-01-02 00:00:00"},
		{"specific-dom", "0 0 0 15 * *", "2026-01-16 00:00:00", "2026-02-15 00:00:00"},
		{"month-name", "0 0 0 1 JUN *", "2026-01-01 00:00:00", "2026-06-01 00:00:00"},
		{"dow-name-monday", "0 0 0 * * MON", "2026-07-17 00:00:00", "2026-07-20 00:00:00"},
		{"dow-number-sunday-0", "0 0 0 * * 0", "2026-07-17 00:00:00", "2026-07-19 00:00:00"},
		{"dow-number-sunday-7", "0 0 0 * * 7", "2026-07-17 00:00:00", "2026-07-19 00:00:00"},
		{"step-range", "0 10-30/10 * * * *", "2026-01-01 10:00:00", "2026-01-01 10:10:00"},
		{"bare-value-step", "0 5/20 * * * *", "2026-01-01 10:06:00", "2026-01-01 10:25:00"},
		{"leap-day", "0 0 0 29 2 *", "2026-01-01 00:00:00", "2028-02-29 00:00:00"},
		{"end-of-month-31", "0 0 0 31 * *", "2026-02-01 00:00:00", "2026-03-31 00:00:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ce := MustParseCron(tc.expr)
			got := ce.Next(mustTime(t, tc.from))
			want := mustTime(t, tc.want)
			if !got.Equal(want) {
				t.Fatalf("Next(%s) = %v, want %v", tc.from, got, want)
			}
		})
	}
}

func TestCronDayOfWeekAndDayOfMonthOr(t *testing.T) {
	// When both DOM and DOW are restricted, match is OR: the 1st of the
	// month OR any Friday.
	ce := MustParseCron("0 0 0 1 * FRI")
	// 2026-07-17 is a Friday; next matching day at/after 2026-07-15 is Fri
	// 2026-07-17.
	got := ce.Next(mustTime(t, "2026-07-15 00:00:00"))
	want := mustTime(t, "2026-07-17 00:00:00")
	if !got.Equal(want) {
		t.Fatalf("Next = %v, want %v", got, want)
	}
}

func TestCronStrictlyAfter(t *testing.T) {
	ce := MustParseCron("0 0 0 * * *")
	// Exactly on a fire time must return the following occurrence.
	got := ce.Next(mustTime(t, "2026-01-01 00:00:00"))
	want := mustTime(t, "2026-01-02 00:00:00")
	if !got.Equal(want) {
		t.Fatalf("Next = %v, want %v", got, want)
	}
}

func TestCronImpossibleSchedule(t *testing.T) {
	// February 30th never occurs; Next should give up and return zero.
	ce := MustParseCron("0 0 0 30 2 *")
	if got := ce.Next(mustTime(t, "2026-01-01 00:00:00")); !got.IsZero() {
		t.Fatalf("Next = %v, want zero time", got)
	}
}

func TestCronSequence(t *testing.T) {
	ce := MustParseCron("0 0 12 * * MON-FRI")
	cur := mustTime(t, "2026-07-17 13:00:00") // Friday afternoon
	want := []string{
		"2026-07-20 12:00:00", // Monday
		"2026-07-21 12:00:00", // Tuesday
		"2026-07-22 12:00:00", // Wednesday
	}
	for _, w := range want {
		cur = ce.Next(cur)
		if !cur.Equal(mustTime(t, w)) {
			t.Fatalf("got %v, want %v", cur, w)
		}
	}
}

func TestCronTimezone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	trig, err := NewCronTrigger("t", "j", "0 30 9 * * *")
	if err != nil {
		t.Fatal(err)
	}
	trig.In(ny)
	// Reference is a UTC instant; fire should be 09:30 New York time.
	from := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	next := trig.ComputeFirstFireTime(from)
	inNY := next.In(ny)
	if inNY.Hour() != 9 || inNY.Minute() != 30 {
		t.Fatalf("expected 09:30 NY, got %v", inNY)
	}
}
