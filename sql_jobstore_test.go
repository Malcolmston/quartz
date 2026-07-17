package quartz

import (
	"errors"
	"testing"
	"time"
)

// sqlJobStoreFakeTrigger is a minimal Trigger implementation used to exercise
// the unsupported-type error path of the SQL trigger serializer.
type sqlJobStoreFakeTrigger struct{}

func (sqlJobStoreFakeTrigger) Key() Key                                 { return NewKey("fake") }
func (sqlJobStoreFakeTrigger) JobKey() Key                              { return NewKey("job") }
func (sqlJobStoreFakeTrigger) ComputeFirstFireTime(time.Time) time.Time { return time.Time{} }
func (sqlJobStoreFakeTrigger) NextFireTime() time.Time                  { return time.Time{} }
func (sqlJobStoreFakeTrigger) PreviousFireTime() time.Time              { return time.Time{} }
func (sqlJobStoreFakeTrigger) Triggered(time.Time) time.Time            { return time.Time{} }
func (sqlJobStoreFakeTrigger) WillFireAgain() bool                      { return false }
func (sqlJobStoreFakeTrigger) MisfirePolicy() MisfirePolicy             { return MisfireSmart }
func (sqlJobStoreFakeTrigger) UpdateAfterMisfire(time.Time) time.Time   { return time.Time{} }
func (sqlJobStoreFakeTrigger) Description() string                      { return "" }

func TestSQLJobStoreTriggerRoundTrip(t *testing.T) {
	start := mustTime(t, "2026-01-01 00:00:00")

	simple := NewSimpleTrigger(NewKey("s"), NewKeyInGroup("j", "g"), start, time.Minute, 3).
		WithDescription("simple").
		WithEndTime(start.Add(time.Hour)).
		WithMisfirePolicy(MisfireDoNothing)
	simple.ComputeFirstFireTime(start)
	simple.Triggered(start)

	cron, err := NewCronTrigger("c", "j", "0 30 * * * *")
	if err != nil {
		t.Fatalf("build cron: %v", err)
	}
	cron.WithDescription("cron").StartingAt(start).EndingAt(start.Add(24 * time.Hour))
	cron.ComputeFirstFireTime(start)
	cron.Triggered(start)

	tests := []struct {
		name string
		in   Trigger
		kind string
	}{
		{"simple", simple, sqlJobStoreKindSimple},
		{"cron", cron, sqlJobStoreKindCron},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, payload, err := sqlJobStoreEncodeTrigger(tc.in)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if kind != tc.kind {
				t.Fatalf("kind = %q, want %q", kind, tc.kind)
			}
			out, err := sqlJobStoreDecodeTrigger(kind, payload)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out.Key() != tc.in.Key() {
				t.Errorf("Key = %v, want %v", out.Key(), tc.in.Key())
			}
			if out.JobKey() != tc.in.JobKey() {
				t.Errorf("JobKey = %v, want %v", out.JobKey(), tc.in.JobKey())
			}
			if out.Description() != tc.in.Description() {
				t.Errorf("Description = %q, want %q", out.Description(), tc.in.Description())
			}
			if !out.NextFireTime().Equal(tc.in.NextFireTime()) {
				t.Errorf("NextFireTime = %v, want %v", out.NextFireTime(), tc.in.NextFireTime())
			}
			if !out.PreviousFireTime().Equal(tc.in.PreviousFireTime()) {
				t.Errorf("PreviousFireTime = %v, want %v", out.PreviousFireTime(), tc.in.PreviousFireTime())
			}
			if out.MisfirePolicy() != tc.in.MisfirePolicy() {
				t.Errorf("MisfirePolicy = %v, want %v", out.MisfirePolicy(), tc.in.MisfirePolicy())
			}
		})
	}
}

func TestSQLJobStoreSimpleTriggerFieldsPreserved(t *testing.T) {
	start := mustTime(t, "2026-03-04 05:06:07")
	in := NewSimpleTrigger(NewKey("s"), NewKey("j"), start, 90*time.Second, 5)
	in.ComputeFirstFireTime(start)
	in.Triggered(start)
	in.Triggered(in.NextFireTime())

	kind, payload, err := sqlJobStoreEncodeTrigger(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := sqlJobStoreDecodeTrigger(kind, payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, ok := decoded.(*SimpleTrigger)
	if !ok {
		t.Fatalf("decoded type = %T, want *SimpleTrigger", decoded)
	}
	if out.Interval() != in.Interval() {
		t.Errorf("Interval = %v, want %v", out.Interval(), in.Interval())
	}
	if out.RepeatCount() != in.RepeatCount() {
		t.Errorf("RepeatCount = %v, want %v", out.RepeatCount(), in.RepeatCount())
	}
	if out.TimesTriggered() != in.TimesTriggered() {
		t.Errorf("TimesTriggered = %v, want %v", out.TimesTriggered(), in.TimesTriggered())
	}
	if !out.StartTime().Equal(in.StartTime()) {
		t.Errorf("StartTime = %v, want %v", out.StartTime(), in.StartTime())
	}
	if !out.endTime.Equal(in.endTime) {
		t.Errorf("endTime = %v, want %v", out.endTime, in.endTime)
	}
}

func TestSQLJobStoreCronExpressionPreserved(t *testing.T) {
	in, err := NewCronTrigger("c", "j", "15 0/5 * * * *")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	in.In(loc)
	in.ComputeFirstFireTime(mustTime(t, "2026-01-01 00:00:00"))

	kind, payload, err := sqlJobStoreEncodeTrigger(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := sqlJobStoreDecodeTrigger(kind, payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out := decoded.(*CronTrigger)
	if out.Expression().String() != in.Expression().String() {
		t.Errorf("expr = %q, want %q", out.Expression().String(), in.Expression().String())
	}
	if out.location.String() != in.location.String() {
		t.Errorf("location = %q, want %q", out.location.String(), in.location.String())
	}
}

func TestSQLJobStoreEncodeTriggerUnknownType(t *testing.T) {
	_, _, err := sqlJobStoreEncodeTrigger(sqlJobStoreFakeTrigger{})
	if !errors.Is(err, sqlJobStoreErrUnknownTrigger) {
		t.Fatalf("err = %v, want wrap of sqlJobStoreErrUnknownTrigger", err)
	}
}

func TestSQLJobStoreDecodeTriggerUnknownKind(t *testing.T) {
	_, err := sqlJobStoreDecodeTrigger("NOPE", nil)
	if !errors.Is(err, sqlJobStoreErrUnknownTrigger) {
		t.Fatalf("err = %v, want wrap of sqlJobStoreErrUnknownTrigger", err)
	}
}

func TestSQLJobStoreJobTypeNameAndRegistry(t *testing.T) {
	if got := jobTypeName(nil); got != "" {
		t.Fatalf("jobTypeName(nil) = %q, want empty", got)
	}
	name := jobTypeName(&countingJob{})
	if name != "*quartz.countingJob" {
		t.Fatalf("jobTypeName = %q, want %q", name, "*quartz.countingJob")
	}

	RegisterJobType(name, func() Job { return &countingJob{} })
	factory := sqlJobStoreLookupJobType(name)
	if factory == nil {
		t.Fatal("factory not registered")
	}
	if _, ok := factory().(*countingJob); !ok {
		t.Fatalf("factory produced %T, want *countingJob", factory())
	}
	if sqlJobStoreLookupJobType("missing") != nil {
		t.Fatal("unexpected factory for missing type")
	}
}

func TestSQLJobStoreDataRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   JobDataMap
	}{
		{"nil", nil},
		{"empty", JobDataMap{}},
		{"mixed", JobDataMap{
			"s": "hello",
			"i": 42,
			"b": true,
			"f": 3.5,
			"t": mustTime(t, "2026-05-06 07:08:09"),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := sqlJobStoreEncodeData(tc.in)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			out, err := sqlJobStoreDecodeData(blob)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out == nil {
				t.Fatal("decoded map is nil")
			}
			if len(out) != len(tc.in) {
				t.Fatalf("len = %d, want %d", len(out), len(tc.in))
			}
			for k, want := range tc.in {
				got := out[k]
				if wt, ok := want.(time.Time); ok {
					if !got.(time.Time).Equal(wt) {
						t.Errorf("key %q time = %v, want %v", k, got, wt)
					}
					continue
				}
				if got != want {
					t.Errorf("key %q = %v, want %v", k, got, want)
				}
			}
		})
	}
}

func TestSQLJobStoreRebind(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		query  string
		want   string
	}{
		{"default", "sqlite", "a=? AND b=?", "a=? AND b=?"},
		{"mysql", "mysql", "a=? AND b=?", "a=? AND b=?"},
		{"postgres", "postgres", "a=? AND b=?", "a=$1 AND b=$2"},
		{"pgx", "pgx", "x IN (?, ?, ?)", "x IN ($1, $2, $3)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sqlJobStoreRebind(tc.driver, tc.query); got != tc.want {
				t.Fatalf("rebind = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSQLJobStoreSchemaStatements(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		prefix string
		blob   string
		quote  string
	}{
		{"sqlite", "sqlite", "qz_", "BLOB", `"group"`},
		{"postgres", "postgres", "", "BYTEA", `"group"`},
		{"mysql", "mysql", "", "BLOB", "`group`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSQLJobStore(nil, SQLJobStoreOptions{Driver: tc.driver, TablePrefix: tc.prefix})
			stmts := s.sqlJobStoreSchemaStatements()
			if len(stmts) != 2 {
				t.Fatalf("statements = %d, want 2", len(stmts))
			}
			for _, stmt := range stmts {
				if !contains(stmt, "CREATE TABLE IF NOT EXISTS") {
					t.Errorf("missing IF NOT EXISTS: %s", stmt)
				}
				if !contains(stmt, tc.quote) {
					t.Errorf("missing quoted group %q: %s", tc.quote, stmt)
				}
			}
			if !contains(stmts[0], tc.prefix+"jobs") {
				t.Errorf("jobs table name missing prefix %q: %s", tc.prefix, stmts[0])
			}
			if !contains(stmts[1], tc.prefix+"triggers") {
				t.Errorf("triggers table name missing prefix %q: %s", tc.prefix, stmts[1])
			}
			if !contains(stmts[0], tc.blob) {
				t.Errorf("jobs missing blob type %q: %s", tc.blob, stmts[0])
			}
		})
	}
}

func TestSQLJobStoreTimeToNanos(t *testing.T) {
	if got := sqlJobStoreTimeToNanos(time.Time{}); got != 0 {
		t.Fatalf("zero time = %d, want 0", got)
	}
	tm := mustTime(t, "2026-07-08 09:10:11")
	if got := sqlJobStoreTimeToNanos(tm); got != tm.UnixNano() {
		t.Fatalf("nanos = %d, want %d", got, tm.UnixNano())
	}
}

// contains is a tiny substring helper kept local to avoid importing strings in
// the test file for a single use.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
