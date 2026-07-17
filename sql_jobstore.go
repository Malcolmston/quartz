package quartz

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Compile-time assertion that SQLJobStore satisfies the JobStore interface so
// it can be used in place of MemoryJobStore via Options.Store.
var _ JobStore = (*SQLJobStore)(nil)

// sqlJobStoreErrUnknownTrigger is the sentinel wrapped by trigger
// (de)serialization when a Trigger's concrete type is not one of the four
// supported implementations. Callers can test for it with errors.Is.
var sqlJobStoreErrUnknownTrigger = errors.New("quartz: sql_jobstore: unsupported trigger type")

// sqlJobStoreErrUnknownJobType is the sentinel wrapped by GetJob when the job
// type recorded in the database has no factory registered via RegisterJobType.
var sqlJobStoreErrUnknownJobType = errors.New("quartz: sql_jobstore: unregistered job type")

// Trigger kind discriminators stored in the triggers.type column. They identify
// which concrete Trigger implementation a payload row was serialized from.
const (
	sqlJobStoreKindSimple   = "SIMPLE"
	sqlJobStoreKindCron     = "CRON"
	sqlJobStoreKindCalendar = "CALENDAR"
	sqlJobStoreKindDaily    = "DAILY"
)

// The job type factory registry maps a job type name (as produced by
// jobTypeName) to a constructor that returns a fresh, zero valued Job of that
// concrete type. Because Job is a non-serializable interface, the SQL store
// records only the type name of a job and reconstructs the instance from the
// registered factory, rehydrating its JobDataMap from the persisted BLOB.
var (
	sqlJobStoreFactoryMu sync.RWMutex
	sqlJobStoreFactories = make(map[string]func() Job)
)

func init() {
	// Register the concrete trigger types so gob can round-trip them if they
	// ever appear inside an interface valued JobDataMap entry, and register
	// the common concrete value types carried by a JobDataMap so gob can
	// encode them through the map[string]any interface values.
	gob.Register(&SimpleTrigger{})
	gob.Register(&CronTrigger{})
	gob.Register(&CalendarIntervalTrigger{})
	gob.Register(&DailyTimeIntervalTrigger{})

	gob.Register(int(0))
	gob.Register(int64(0))
	gob.Register(float64(0))
	gob.Register(false)
	gob.Register("")
	gob.Register([]byte(nil))
	gob.Register(time.Time{})
	gob.Register(time.Duration(0))
	gob.Register(map[string]any{})
	gob.Register([]any(nil))
}

// RegisterJobType registers a factory that constructs a fresh Job of a concrete
// type. The name must match the value returned by jobTypeName for instances of
// that type (that is, reflect.TypeOf(job).String(), such as
// "*mypkg.EmailJob"), because the SQL store persists a job by that name and
// rebuilds it by looking the factory up again. Registering the same name twice
// replaces the previous factory. It is safe for concurrent use.
func RegisterJobType(name string, factory func() Job) {
	sqlJobStoreFactoryMu.Lock()
	defer sqlJobStoreFactoryMu.Unlock()
	sqlJobStoreFactories[name] = factory
}

// jobTypeName returns the stable type name used to persist and later
// reconstruct a Job. It is reflect.TypeOf(j).String() and returns the empty
// string for a nil Job.
func jobTypeName(j Job) string {
	if j == nil {
		return ""
	}
	return reflect.TypeOf(j).String()
}

// sqlJobStoreLookupJobType returns the factory registered for a job type name,
// or nil if none is registered.
func sqlJobStoreLookupJobType(name string) func() Job {
	sqlJobStoreFactoryMu.RLock()
	defer sqlJobStoreFactoryMu.RUnlock()
	return sqlJobStoreFactories[name]
}

// SQLJobStoreOptions configures a SQLJobStore.
type SQLJobStoreOptions struct {
	// Driver is the name of the database/sql driver backing the supplied
	// *sql.DB (for example "sqlite", "postgres", or "mysql"). It selects the
	// SQL dialect used for identifier quoting, bind placeholders, and column
	// types. An empty value uses a portable default (ANSI quoting and "?"
	// placeholders).
	Driver string
	// TablePrefix is prepended to the "jobs" and "triggers" table names,
	// allowing several stores to share a database. It may be empty.
	TablePrefix string
}

// SQLJobStore is a persistent JobStore backed solely by the standard library
// database/sql package. The caller supplies an already configured *sql.DB; no
// third-party driver is imported by this package. Jobs are persisted by type
// name plus a gob encoded JobDataMap and reconstructed through the factory
// registry (see RegisterJobType); triggers are persisted by serializing the
// unexported state of the four supported concrete Trigger implementations. All
// methods take the store mutex so that operations observe a single,
// connection-pool-agnostic order.
type SQLJobStore struct {
	db     *sql.DB
	driver string
	jobs   string
	trig   string

	mu sync.Mutex
}

// NewSQLJobStore constructs a SQLJobStore over the given *sql.DB and options.
// It does not touch the database; call CreateSchema to create the backing
// tables. The db may be nil only when the store is used solely for the
// stateless (de)serialization helpers.
func NewSQLJobStore(db *sql.DB, opts SQLJobStoreOptions) *SQLJobStore {
	return &SQLJobStore{
		db:     db,
		driver: opts.Driver,
		jobs:   opts.TablePrefix + "jobs",
		trig:   opts.TablePrefix + "triggers",
	}
}

// sqlJobStoreIsPostgres reports whether the driver uses PostgreSQL dialect
// ($N bind placeholders and BYTEA blobs).
func sqlJobStoreIsPostgres(driver string) bool {
	d := strings.ToLower(driver)
	return strings.Contains(d, "postgres") || strings.Contains(d, "pgx")
}

// sqlJobStoreIsMySQL reports whether the driver uses MySQL dialect (backtick
// quoted identifiers).
func sqlJobStoreIsMySQL(driver string) bool {
	d := strings.ToLower(driver)
	return strings.Contains(d, "mysql") || strings.Contains(d, "maria")
}

// sqlJobStoreRebind rewrites the "?" bind placeholders in query to the dialect
// expected by driver. Only PostgreSQL differs, using ordinal $N placeholders.
func sqlJobStoreRebind(driver, query string) string {
	if !sqlJobStoreIsPostgres(driver) {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

// sqlJobStoreQuote returns id quoted as an identifier for the store's dialect.
// It is used for the reserved words "group" and "type".
func (s *SQLJobStore) sqlJobStoreQuote(id string) string {
	if sqlJobStoreIsMySQL(s.driver) {
		return "`" + id + "`"
	}
	return `"` + id + `"`
}

// sqlJobStoreBlobType returns the column type used for BLOB columns in the
// store's dialect.
func (s *SQLJobStore) sqlJobStoreBlobType() string {
	if sqlJobStoreIsPostgres(s.driver) {
		return "BYTEA"
	}
	return "BLOB"
}

// sqlJobStoreExec runs query (after rebinding placeholders) against the store's
// database.
func (s *SQLJobStore) sqlJobStoreExec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, sqlJobStoreRebind(s.driver, query), args...)
}

// sqlJobStoreQueryRow runs a single-row query (after rebinding placeholders).
func (s *SQLJobStore) sqlJobStoreQueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, sqlJobStoreRebind(s.driver, query), args...)
}

// sqlJobStoreQuery runs a multi-row query (after rebinding placeholders).
func (s *SQLJobStore) sqlJobStoreQuery(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, sqlJobStoreRebind(s.driver, query), args...)
}

// sqlJobStoreSchemaStatements returns the idempotent DDL statements that create
// the jobs and triggers tables for the store's dialect.
func (s *SQLJobStore) sqlJobStoreSchemaStatements() []string {
	group := s.sqlJobStoreQuote("group")
	typ := s.sqlJobStoreQuote("type")
	blob := s.sqlJobStoreBlobType()

	jobs := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	name TEXT NOT NULL,
	%s TEXT NOT NULL,
	description TEXT,
	durable INTEGER NOT NULL,
	job_type TEXT NOT NULL,
	job_data %s,
	PRIMARY KEY (name, %s)
)`, s.jobs, group, blob, group)

	triggers := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	name TEXT NOT NULL,
	%s TEXT NOT NULL,
	job_name TEXT NOT NULL,
	job_group TEXT NOT NULL,
	%s TEXT NOT NULL,
	state INTEGER NOT NULL,
	next_fire BIGINT NOT NULL,
	prev_fire BIGINT NOT NULL,
	description TEXT,
	payload %s,
	PRIMARY KEY (name, %s)
)`, s.trig, group, typ, blob, group)

	return []string{jobs, triggers}
}

// CreateSchema creates the jobs and triggers tables if they do not already
// exist. It is idempotent and safe to call on every startup.
func (s *SQLJobStore) CreateSchema(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stmt := range s.sqlJobStoreSchemaStatements() {
		if _, err := s.sqlJobStoreExec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// ---- gob helpers -----------------------------------------------------------

// sqlJobStoreGobEncode gob-encodes v into a byte slice.
func sqlJobStoreGobEncode(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sqlJobStoreGobDecode gob-decodes data into the value pointed to by v.
func sqlJobStoreGobDecode(data []byte, v any) error {
	return gob.NewDecoder(bytes.NewReader(data)).Decode(v)
}

// sqlJobStoreEncodeData gob-encodes a JobDataMap to its BLOB representation. A
// nil map is encoded as an empty map so decoding always yields a usable value.
func sqlJobStoreEncodeData(data JobDataMap) ([]byte, error) {
	m := map[string]any(data)
	if m == nil {
		m = map[string]any{}
	}
	return sqlJobStoreGobEncode(m)
}

// sqlJobStoreDecodeData gob-decodes a JobDataMap from its BLOB representation.
// Empty input yields an empty, non-nil map.
func sqlJobStoreDecodeData(blob []byte) (JobDataMap, error) {
	if len(blob) == 0 {
		return JobDataMap{}, nil
	}
	m := map[string]any{}
	if err := sqlJobStoreGobDecode(blob, &m); err != nil {
		return nil, err
	}
	return JobDataMap(m), nil
}

// sqlJobStoreLoadLocation resolves a location name previously produced by
// (*time.Location).String() back to a *time.Location.
func sqlJobStoreLoadLocation(name string) (*time.Location, error) {
	switch name {
	case "", "UTC":
		return time.UTC, nil
	case "Local":
		return time.Local, nil
	default:
		return time.LoadLocation(name)
	}
}

// ---- trigger wire structs --------------------------------------------------
//
// Each wire struct mirrors the unexported state of one concrete Trigger using
// exported fields so gob can encode it. Times are stored as time.Time (which
// gob round-trips), and locations as their String() name. The wire structs for
// CalendarIntervalTrigger and DailyTimeIntervalTrigger describe the unexported
// fields those (separately implemented) trigger types are expected to carry;
// their interval unit is persisted as the underlying int of the shared
// IntervalUnit enum.

// sqlJobStoreSimpleWire is the serialized form of a *SimpleTrigger.
type sqlJobStoreSimpleWire struct {
	Key            Key
	JobKey         Key
	Description    string
	StartTime      time.Time
	EndTime        time.Time
	Interval       time.Duration
	RepeatCount    int
	MisfirePolicy  MisfirePolicy
	TimesTriggered int
	Next           time.Time
	Prev           time.Time
}

// sqlJobStoreCronWire is the serialized form of a *CronTrigger.
type sqlJobStoreCronWire struct {
	Key           Key
	JobKey        Key
	Description   string
	Source        string
	Location      string
	StartTime     time.Time
	EndTime       time.Time
	MisfirePolicy MisfirePolicy
	Next          time.Time
	Prev          time.Time
}

// sqlJobStoreCalendarWire is the serialized form of a *CalendarIntervalTrigger.
type sqlJobStoreCalendarWire struct {
	Key            Key
	JobKey         Key
	Description    string
	StartTime      time.Time
	EndTime        time.Time
	Interval       int
	Unit           int
	Location       string
	MisfirePolicy  MisfirePolicy
	TimesTriggered int
	Next           time.Time
	Prev           time.Time
}

// sqlJobStoreDailyWire is the serialized form of a *DailyTimeIntervalTrigger.
type sqlJobStoreDailyWire struct {
	Key            Key
	JobKey         Key
	Description    string
	StartTime      time.Time
	EndTime        time.Time
	StartTimeOfDay time.Duration
	EndTimeOfDay   time.Duration
	DaysOfWeek     []time.Weekday
	Interval       int
	Unit           int
	RepeatCount    int
	Location       string
	MisfirePolicy  MisfirePolicy
	TimesTriggered int
	Next           time.Time
	Prev           time.Time
}

// sqlJobStoreEncodeTrigger serializes a Trigger to a kind discriminator and a
// gob encoded payload by type-switching on the concrete pointer type and
// reading its unexported fields directly (legal in-package). Unknown trigger
// types yield an error wrapping sqlJobStoreErrUnknownTrigger.
func sqlJobStoreEncodeTrigger(trig Trigger) (kind string, payload []byte, err error) {
	switch t := trig.(type) {
	case *SimpleTrigger:
		payload, err = sqlJobStoreGobEncode(sqlJobStoreSimpleWire{
			Key:            t.key,
			JobKey:         t.jobKey,
			Description:    t.description,
			StartTime:      t.startTime,
			EndTime:        t.endTime,
			Interval:       t.interval,
			RepeatCount:    t.repeatCount,
			MisfirePolicy:  t.misfirePolicy,
			TimesTriggered: t.timesTriggered,
			Next:           t.next,
			Prev:           t.prev,
		})
		return sqlJobStoreKindSimple, payload, err

	case *CronTrigger:
		payload, err = sqlJobStoreGobEncode(sqlJobStoreCronWire{
			Key:           t.key,
			JobKey:        t.jobKey,
			Description:   t.description,
			Source:        t.expr.String(),
			Location:      t.location.String(),
			StartTime:     t.startTime,
			EndTime:       t.endTime,
			MisfirePolicy: t.misfirePolicy,
			Next:          t.next,
			Prev:          t.prev,
		})
		return sqlJobStoreKindCron, payload, err

	case *CalendarIntervalTrigger:
		payload, err = sqlJobStoreGobEncode(sqlJobStoreCalendarWire{
			Key:            t.key,
			JobKey:         t.jobKey,
			Description:    t.description,
			StartTime:      t.startTime,
			EndTime:        t.endTime,
			Interval:       t.count,
			Unit:           int(t.unit),
			Location:       t.location.String(),
			MisfirePolicy:  t.misfirePolicy,
			TimesTriggered: t.timesTriggered,
			Next:           t.next,
			Prev:           t.prev,
		})
		return sqlJobStoreKindCalendar, payload, err

	case *DailyTimeIntervalTrigger:
		payload, err = sqlJobStoreGobEncode(sqlJobStoreDailyWire{
			Key:            t.key,
			JobKey:         t.jobKey,
			Description:    t.description,
			StartTime:      t.startTime,
			EndTime:        t.endTime,
			StartTimeOfDay: t.startTimeOfDay,
			EndTimeOfDay:   t.endTimeOfDay,
			DaysOfWeek:     t.daysOfWeek,
			Interval:       t.interval,
			Unit:           int(t.unit),
			RepeatCount:    t.repeatCount,
			Location:       t.location.String(),
			MisfirePolicy:  t.misfirePolicy,
			TimesTriggered: t.timesTriggered,
			Next:           t.next,
			Prev:           t.prev,
		})
		return sqlJobStoreKindDaily, payload, err

	default:
		return "", nil, fmt.Errorf("%w: %T", sqlJobStoreErrUnknownTrigger, trig)
	}
}

// sqlJobStoreDecodeTrigger reconstructs a Trigger from its kind discriminator
// and gob payload, writing the persisted state back into the concrete type's
// unexported fields. An unrecognized kind yields an error wrapping
// sqlJobStoreErrUnknownTrigger.
func sqlJobStoreDecodeTrigger(kind string, payload []byte) (Trigger, error) {
	switch kind {
	case sqlJobStoreKindSimple:
		var w sqlJobStoreSimpleWire
		if err := sqlJobStoreGobDecode(payload, &w); err != nil {
			return nil, err
		}
		return &SimpleTrigger{
			key:            w.Key,
			jobKey:         w.JobKey,
			description:    w.Description,
			startTime:      w.StartTime,
			endTime:        w.EndTime,
			interval:       w.Interval,
			repeatCount:    w.RepeatCount,
			misfirePolicy:  w.MisfirePolicy,
			timesTriggered: w.TimesTriggered,
			next:           w.Next,
			prev:           w.Prev,
		}, nil

	case sqlJobStoreKindCron:
		var w sqlJobStoreCronWire
		if err := sqlJobStoreGobDecode(payload, &w); err != nil {
			return nil, err
		}
		expr, err := ParseCron(w.Source)
		if err != nil {
			return nil, err
		}
		loc, err := sqlJobStoreLoadLocation(w.Location)
		if err != nil {
			return nil, err
		}
		return &CronTrigger{
			key:           w.Key,
			jobKey:        w.JobKey,
			description:   w.Description,
			expr:          expr,
			location:      loc,
			startTime:     w.StartTime,
			endTime:       w.EndTime,
			misfirePolicy: w.MisfirePolicy,
			next:          w.Next,
			prev:          w.Prev,
		}, nil

	case sqlJobStoreKindCalendar:
		var w sqlJobStoreCalendarWire
		if err := sqlJobStoreGobDecode(payload, &w); err != nil {
			return nil, err
		}
		loc, err := sqlJobStoreLoadLocation(w.Location)
		if err != nil {
			return nil, err
		}
		return &CalendarIntervalTrigger{
			key:            w.Key,
			jobKey:         w.JobKey,
			description:    w.Description,
			startTime:      w.StartTime,
			endTime:        w.EndTime,
			count:          w.Interval,
			unit:           IntervalUnit(w.Unit),
			location:       loc,
			misfirePolicy:  w.MisfirePolicy,
			timesTriggered: w.TimesTriggered,
			next:           w.Next,
			prev:           w.Prev,
		}, nil

	case sqlJobStoreKindDaily:
		var w sqlJobStoreDailyWire
		if err := sqlJobStoreGobDecode(payload, &w); err != nil {
			return nil, err
		}
		loc, err := sqlJobStoreLoadLocation(w.Location)
		if err != nil {
			return nil, err
		}
		return &DailyTimeIntervalTrigger{
			key:            w.Key,
			jobKey:         w.JobKey,
			description:    w.Description,
			startTime:      w.StartTime,
			endTime:        w.EndTime,
			startTimeOfDay: w.StartTimeOfDay,
			endTimeOfDay:   w.EndTimeOfDay,
			daysOfWeek:     w.DaysOfWeek,
			interval:       w.Interval,
			unit:           IntervalUnit(w.Unit),
			repeatCount:    w.RepeatCount,
			location:       loc,
			misfirePolicy:  w.MisfirePolicy,
			timesTriggered: w.TimesTriggered,
			next:           w.Next,
			prev:           w.Prev,
		}, nil

	default:
		return nil, fmt.Errorf("%w: %q", sqlJobStoreErrUnknownTrigger, kind)
	}
}

// sqlJobStoreTimeToNanos converts a time to the nanosecond value stored in the
// next_fire/prev_fire columns. The zero time maps to 0 so it can be excluded by
// the AcquireNextTriggers query with next_fire > 0.
func sqlJobStoreTimeToNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// ---- JobStore: jobs --------------------------------------------------------

// StoreJob implements JobStore. It persists the JobDetail's identity, type
// name, and gob encoded JobDataMap. If replace is false and a job with the same
// key already exists, ErrJobExists is returned.
func (s *SQLJobStore) StoreJob(detail *JobDetail, replace bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()

	data, err := sqlJobStoreEncodeData(detail.data)
	if err != nil {
		return err
	}
	key := detail.key

	exists, err := s.sqlJobStoreJobExists(ctx, key)
	if err != nil {
		return err
	}
	if exists && !replace {
		return ErrJobExists
	}

	group := s.sqlJobStoreQuote("group")
	durable := 0
	if detail.durable {
		durable = 1
	}
	if exists {
		q := fmt.Sprintf(`UPDATE %s SET description=?, durable=?, job_type=?, job_data=? WHERE name=? AND %s=?`,
			s.jobs, group)
		_, err = s.sqlJobStoreExec(ctx, q, detail.description, durable, jobTypeName(detail.job), data, key.Name, key.Group)
		return err
	}
	q := fmt.Sprintf(`INSERT INTO %s (name, %s, description, durable, job_type, job_data) VALUES (?, ?, ?, ?, ?, ?)`,
		s.jobs, group)
	_, err = s.sqlJobStoreExec(ctx, q, key.Name, key.Group, detail.description, durable, jobTypeName(detail.job), data)
	return err
}

// sqlJobStoreJobExists reports whether a job row with the given key exists.
func (s *SQLJobStore) sqlJobStoreJobExists(ctx context.Context, key Key) (bool, error) {
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`SELECT 1 FROM %s WHERE name=? AND %s=?`, s.jobs, group)
	var one int
	err := s.sqlJobStoreQueryRow(ctx, q, key.Name, key.Group).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// RemoveJob implements JobStore. It reports whether a job row was deleted.
func (s *SQLJobStore) RemoveJob(key Key) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`DELETE FROM %s WHERE name=? AND %s=?`, s.jobs, group)
	res, err := s.sqlJobStoreExec(context.Background(), q, key.Name, key.Group)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// GetJob implements JobStore. It reconstructs the Job through the registered
// factory (see RegisterJobType) and rehydrates its JobDataMap. It returns
// ErrJobNotFound when no row exists.
func (s *SQLJobStore) GetJob(key Key) (*JobDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`SELECT description, durable, job_type, job_data FROM %s WHERE name=? AND %s=?`,
		s.jobs, group)

	var (
		desc    sql.NullString
		durable int
		jobType string
		data    []byte
	)
	err := s.sqlJobStoreQueryRow(context.Background(), q, key.Name, key.Group).
		Scan(&desc, &durable, &jobType, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, err
	}

	factory := sqlJobStoreLookupJobType(jobType)
	if factory == nil {
		return nil, fmt.Errorf("%w: %s", sqlJobStoreErrUnknownJobType, jobType)
	}
	dataMap, err := sqlJobStoreDecodeData(data)
	if err != nil {
		return nil, err
	}
	return &JobDetail{
		key:         key,
		description: desc.String,
		job:         factory(),
		data:        dataMap,
		durable:     durable != 0,
	}, nil
}

// JobKeys implements JobStore. Keys are ordered by group then name.
func (s *SQLJobStore) JobKeys() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`SELECT name, %s FROM %s ORDER BY %s, name`, group, s.jobs, group)
	rows, err := s.sqlJobStoreQuery(context.Background(), q)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var keys []Key
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.Name, &k.Group); err != nil {
			return keys
		}
		keys = append(keys, k)
	}
	return keys
}

// ---- JobStore: triggers ----------------------------------------------------

// StoreTrigger implements JobStore. It serializes the trigger's concrete state
// and persists it in the given state. If replace is false and a trigger with
// the same key already exists, ErrTriggerExists is returned.
func (s *SQLJobStore) StoreTrigger(trigger Trigger, state TriggerState, replace bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()

	kind, payload, err := sqlJobStoreEncodeTrigger(trigger)
	if err != nil {
		return err
	}
	key := trigger.Key()
	jobKey := trigger.JobKey()

	exists, err := s.sqlJobStoreTriggerExists(ctx, key)
	if err != nil {
		return err
	}
	if exists && !replace {
		return ErrTriggerExists
	}

	group := s.sqlJobStoreQuote("group")
	typ := s.sqlJobStoreQuote("type")
	next := sqlJobStoreTimeToNanos(trigger.NextFireTime())
	prev := sqlJobStoreTimeToNanos(trigger.PreviousFireTime())

	if exists {
		q := fmt.Sprintf(`UPDATE %s SET job_name=?, job_group=?, %s=?, state=?, next_fire=?, prev_fire=?, description=?, payload=? WHERE name=? AND %s=?`,
			s.trig, typ, group)
		_, err = s.sqlJobStoreExec(ctx, q,
			jobKey.Name, jobKey.Group, kind, int(state), next, prev, trigger.Description(), payload,
			key.Name, key.Group)
		return err
	}
	q := fmt.Sprintf(`INSERT INTO %s (name, %s, job_name, job_group, %s, state, next_fire, prev_fire, description, payload) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.trig, group, typ)
	_, err = s.sqlJobStoreExec(ctx, q,
		key.Name, key.Group, jobKey.Name, jobKey.Group, kind, int(state), next, prev, trigger.Description(), payload)
	return err
}

// sqlJobStoreTriggerExists reports whether a trigger row with the given key
// exists.
func (s *SQLJobStore) sqlJobStoreTriggerExists(ctx context.Context, key Key) (bool, error) {
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`SELECT 1 FROM %s WHERE name=? AND %s=?`, s.trig, group)
	var one int
	err := s.sqlJobStoreQueryRow(ctx, q, key.Name, key.Group).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// RemoveTrigger implements JobStore. It reports whether a trigger row was
// deleted.
func (s *SQLJobStore) RemoveTrigger(key Key) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`DELETE FROM %s WHERE name=? AND %s=?`, s.trig, group)
	res, err := s.sqlJobStoreExec(context.Background(), q, key.Name, key.Group)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// GetTrigger implements JobStore. It returns ErrTriggerNotFound when no row
// exists.
func (s *SQLJobStore) GetTrigger(key Key) (*StoredTrigger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	typ := s.sqlJobStoreQuote("type")
	q := fmt.Sprintf(`SELECT %s, state, payload FROM %s WHERE name=? AND %s=?`, typ, s.trig, group)

	var (
		kind    string
		state   int
		payload []byte
	)
	err := s.sqlJobStoreQueryRow(context.Background(), q, key.Name, key.Group).Scan(&kind, &state, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTriggerNotFound
	}
	if err != nil {
		return nil, err
	}
	trig, err := sqlJobStoreDecodeTrigger(kind, payload)
	if err != nil {
		return nil, err
	}
	return &StoredTrigger{Trigger: trig, State: TriggerState(state)}, nil
}

// TriggerKeys implements JobStore. Keys are ordered by group then name.
func (s *SQLJobStore) TriggerKeys() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`SELECT name, %s FROM %s ORDER BY %s, name`, group, s.trig, group)
	rows, err := s.sqlJobStoreQuery(context.Background(), q)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var keys []Key
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.Name, &k.Group); err != nil {
			return keys
		}
		keys = append(keys, k)
	}
	return keys
}

// TriggersForJob implements JobStore. It returns every stored trigger that
// references the given job, ordered by group then name.
func (s *SQLJobStore) TriggersForJob(jobKey Key) []*StoredTrigger {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	typ := s.sqlJobStoreQuote("type")
	q := fmt.Sprintf(`SELECT %s, state, payload FROM %s WHERE job_name=? AND job_group=? ORDER BY %s, name`,
		typ, s.trig, group)
	rows, err := s.sqlJobStoreQuery(context.Background(), q, jobKey.Name, jobKey.Group)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*StoredTrigger
	for rows.Next() {
		var (
			kind    string
			state   int
			payload []byte
		)
		if err := rows.Scan(&kind, &state, &payload); err != nil {
			return out
		}
		trig, err := sqlJobStoreDecodeTrigger(kind, payload)
		if err != nil {
			continue
		}
		out = append(out, &StoredTrigger{Trigger: trig, State: TriggerState(state)})
	}
	return out
}

// SetTriggerState implements JobStore. It returns ErrTriggerNotFound when no
// row matches the key.
func (s *SQLJobStore) SetTriggerState(key Key, state TriggerState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	q := fmt.Sprintf(`UPDATE %s SET state=? WHERE name=? AND %s=?`, s.trig, group)
	res, err := s.sqlJobStoreExec(context.Background(), q, int(state), key.Name, key.Group)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTriggerNotFound
	}
	return nil
}

// AcquireNextTriggers implements JobStore. It returns up to maxCount triggers in
// the NORMAL state whose next fire time is at or before noLaterThan, ordered by
// fire time. A maxCount of zero or less imposes no limit. It does not mutate any
// state.
func (s *SQLJobStore) AcquireNextTriggers(noLaterThan time.Time, maxCount int) []*StoredTrigger {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.sqlJobStoreQuote("group")
	typ := s.sqlJobStoreQuote("type")

	q := fmt.Sprintf(`SELECT %s, state, payload FROM %s WHERE state=? AND next_fire<=? AND next_fire>0 ORDER BY next_fire, %s, name`,
		typ, s.trig, group)
	args := []any{int(TriggerStateNormal), sqlJobStoreTimeToNanos(noLaterThan)}
	if maxCount > 0 {
		q += " LIMIT ?"
		args = append(args, maxCount)
	}

	rows, err := s.sqlJobStoreQuery(context.Background(), q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*StoredTrigger
	for rows.Next() {
		var (
			kind    string
			state   int
			payload []byte
		)
		if err := rows.Scan(&kind, &state, &payload); err != nil {
			return out
		}
		trig, err := sqlJobStoreDecodeTrigger(kind, payload)
		if err != nil {
			continue
		}
		out = append(out, &StoredTrigger{Trigger: trig, State: TriggerState(state)})
	}
	return out
}
