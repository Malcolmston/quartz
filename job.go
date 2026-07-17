package quartz

import (
	"context"
	"fmt"
)

// Job is the unit of work executed by the scheduler. Implementations must be
// safe for concurrent use if the same instance is shared across multiple
// JobDetails or triggers. The provided context is cancelled when the scheduler
// is shutting down without waiting for the job to finish.
type Job interface {
	Execute(ctx context.Context) error
}

// JobFunc adapts an ordinary function to the Job interface.
type JobFunc func(ctx context.Context) error

// Execute implements the Job interface.
func (f JobFunc) Execute(ctx context.Context) error { return f(ctx) }

// Key uniquely identifies a job or trigger within a group. The zero Key is not
// valid; use NewKey to construct one. When a group is omitted it defaults to
// DefaultGroup.
type Key struct {
	Name  string
	Group string
}

// DefaultGroup is used when a Key is created without an explicit group.
const DefaultGroup = "DEFAULT"

// NewKey builds a Key in the DefaultGroup.
func NewKey(name string) Key { return Key{Name: name, Group: DefaultGroup} }

// NewKeyInGroup builds a Key in the given group. An empty group is replaced
// with DefaultGroup.
func NewKeyInGroup(name, group string) Key {
	if group == "" {
		group = DefaultGroup
	}
	return Key{Name: name, Group: group}
}

// String returns the "group.name" representation of the Key.
func (k Key) String() string { return k.Group + "." + k.Name }

// IsZero reports whether the Key carries no name.
func (k Key) IsZero() bool { return k.Name == "" }

// JobDataMap is a string-keyed bag of state passed to a Job at execution time.
// It is carried both by a JobDetail and, after merging, by every trigger fire.
type JobDataMap map[string]any

// Clone returns a shallow copy of the map so callers can mutate it without
// affecting the original.
func (m JobDataMap) Clone() JobDataMap {
	out := make(JobDataMap, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// GetString returns the value at key as a string and whether it was present and
// of the expected type.
func (m JobDataMap) GetString(key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetInt returns the value at key as an int and whether it was present and of
// the expected type.
func (m JobDataMap) GetInt(key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	i, ok := v.(int)
	return i, ok
}

// JobDetail is the definition of a job: its identity, an optional description,
// the concrete Job implementation to run, and a JobDataMap of state. A single
// JobDetail may be fired by multiple triggers.
type JobDetail struct {
	key         Key
	description string
	job         Job
	data        JobDataMap
	durable     bool
}

// NewJobDetail constructs a JobDetail for the given key and Job.
func NewJobDetail(key Key, job Job) *JobDetail {
	return &JobDetail{
		key:  key,
		job:  job,
		data: JobDataMap{},
	}
}

// WithDescription sets a human readable description and returns the detail for
// chaining.
func (d *JobDetail) WithDescription(desc string) *JobDetail {
	d.description = desc
	return d
}

// WithData sets the JobDataMap and returns the detail for chaining. The map is
// cloned so later external mutation does not leak in.
func (d *JobDetail) WithData(data JobDataMap) *JobDetail {
	d.data = data.Clone()
	return d
}

// Durable marks the job as durable, meaning it is retained by the store even
// when no triggers reference it. Non-durable jobs are removed once their last
// trigger is unscheduled.
func (d *JobDetail) Durable(durable bool) *JobDetail {
	d.durable = durable
	return d
}

// Key returns the job's identity.
func (d *JobDetail) Key() Key { return d.key }

// Description returns the job's description.
func (d *JobDetail) Description() string { return d.description }

// Job returns the concrete Job implementation.
func (d *JobDetail) Job() Job { return d.job }

// Data returns the JobDataMap associated with the job.
func (d *JobDetail) Data() JobDataMap { return d.data }

// IsDurable reports whether the job is durable.
func (d *JobDetail) IsDurable() bool { return d.durable }

// String returns a debug representation of the JobDetail.
func (d *JobDetail) String() string {
	return fmt.Sprintf("JobDetail{key=%s, durable=%t}", d.key, d.durable)
}
