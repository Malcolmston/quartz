package quartz

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpression is a parsed cron schedule. It supports six space separated
// fields:
//
//	second      0-59
//	minute      0-59
//	hour        0-23
//	dayOfMonth  1-31
//	month       1-12 or JAN-DEC
//	dayOfWeek   0-6  or SUN-SAT (0 and 7 both mean Sunday)
//
// A five field expression (without seconds) is also accepted; a "0" seconds
// field is assumed.
//
// Each field supports the following syntax (shown with literal tokens):
//
//	star     every value
//	?        no specific value (treated as star; for dayOfMonth/dayOfWeek)
//	1-5      an inclusive range
//	star/15  a step over the whole range
//	10-30/5  a step over a range
//	1,3,5    a list of values or ranges
//	MON,JAN  three letter names (case insensitive) for month and dayOfWeek
//
// Day matching follows Unix semantics: when both dayOfMonth and dayOfWeek are
// restricted (neither * nor ?), a day matches if either field matches;
// otherwise only the restricted field applies.
type CronExpression struct {
	source string

	seconds    uint64
	minutes    uint64
	hours      uint64
	daysOfMon  uint64
	months     uint64
	daysOfWeek uint64

	domRestricted bool
	dowRestricted bool
}

type fieldSpec struct {
	name  string
	min   int
	max   int
	names map[string]int
}

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var dowNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// ParseCron parses a cron expression. It returns an error describing the first
// problem encountered.
func ParseCron(expr string) (*CronExpression, error) {
	fields := strings.Fields(expr)
	switch len(fields) {
	case 5:
		fields = append([]string{"0"}, fields...)
	case 6:
		// ok
	default:
		return nil, fmt.Errorf("cron: expected 5 or 6 fields, got %d in %q", len(fields), expr)
	}

	ce := &CronExpression{source: expr}
	var err error

	if ce.seconds, _, err = parseField(fields[0], fieldSpec{"second", 0, 59, nil}); err != nil {
		return nil, err
	}
	if ce.minutes, _, err = parseField(fields[1], fieldSpec{"minute", 0, 59, nil}); err != nil {
		return nil, err
	}
	if ce.hours, _, err = parseField(fields[2], fieldSpec{"hour", 0, 23, nil}); err != nil {
		return nil, err
	}
	if ce.daysOfMon, ce.domRestricted, err = parseField(fields[3], fieldSpec{"dayOfMonth", 1, 31, nil}); err != nil {
		return nil, err
	}
	if ce.months, _, err = parseField(fields[4], fieldSpec{"month", 1, 12, monthNames}); err != nil {
		return nil, err
	}
	if ce.daysOfWeek, ce.dowRestricted, err = parseField(fields[5], fieldSpec{"dayOfWeek", 0, 6, dowNames}); err != nil {
		return nil, err
	}
	// Normalize Sunday: allow 7 to mean 0.
	if ce.daysOfWeek&(1<<7) != 0 {
		ce.daysOfWeek |= 1 << 0
		ce.daysOfWeek &^= 1 << 7
	}
	return ce, nil
}

// MustParseCron is like ParseCron but panics on error. It is intended for use
// with constant expressions in tests and package initialization.
func MustParseCron(expr string) *CronExpression {
	ce, err := ParseCron(expr)
	if err != nil {
		panic(err)
	}
	return ce
}

// String returns the original source expression.
func (c *CronExpression) String() string { return c.source }

// parseField parses a single cron field into a bitmask over [min, max]. The
// second return reports whether the field is restricted (i.e. not * and not ?).
func parseField(field string, spec fieldSpec) (uint64, bool, error) {
	if field == "*" || field == "?" {
		return rangeMask(spec.min, spec.max), false, nil
	}

	var mask uint64
	for _, part := range strings.Split(field, ",") {
		m, err := parseListEntry(part, spec)
		if err != nil {
			return 0, false, err
		}
		mask |= m
	}
	return mask, true, nil
}

// parseListEntry parses a single comma separated entry, which may itself carry
// a step and/or a range.
func parseListEntry(part string, spec fieldSpec) (uint64, error) {
	step := 1
	rangePart := part
	if slash := strings.IndexByte(part, '/'); slash >= 0 {
		rangePart = part[:slash]
		stepStr := part[slash+1:]
		s, err := strconv.Atoi(stepStr)
		if err != nil || s <= 0 {
			return 0, fmt.Errorf("cron: invalid step %q in %s field", stepStr, spec.name)
		}
		step = s
	}

	var lo, hi int
	switch {
	case rangePart == "*" || rangePart == "?":
		lo, hi = spec.min, spec.max
	case strings.ContainsRune(rangePart, '-'):
		bounds := strings.SplitN(rangePart, "-", 2)
		var err error
		if lo, err = parseValue(bounds[0], spec); err != nil {
			return 0, err
		}
		if hi, err = parseValue(bounds[1], spec); err != nil {
			return 0, err
		}
	default:
		v, err := parseValue(rangePart, spec)
		if err != nil {
			return 0, err
		}
		lo = v
		// A bare value with a step (e.g. 5/10) ranges from the value to max.
		if step > 1 {
			hi = spec.max
		} else {
			hi = v
		}
	}

	// dayOfWeek permits 7 (Sunday) which is normalized to 0 after parsing.
	checkMax := spec.max
	if spec.name == "dayOfWeek" {
		checkMax = 7
	}
	if lo < spec.min || hi > checkMax || lo > hi {
		return 0, fmt.Errorf("cron: value %d-%d out of range [%d,%d] in %s field", lo, hi, spec.min, spec.max, spec.name)
	}

	var mask uint64
	for v := lo; v <= hi; v += step {
		mask |= 1 << uint(v)
	}
	return mask, nil
}

// parseValue parses a single numeric or named value for a field.
func parseValue(s string, spec fieldSpec) (int, error) {
	s = strings.TrimSpace(s)
	if spec.names != nil {
		if v, ok := spec.names[strings.ToLower(s)]; ok {
			return v, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("cron: invalid value %q in %s field", s, spec.name)
	}
	// Permit 7 for Sunday before normalization.
	if spec.name == "dayOfWeek" && v == 7 {
		return 7, nil
	}
	if v < spec.min || v > spec.max {
		return 0, fmt.Errorf("cron: value %d out of range [%d,%d] in %s field", v, spec.min, spec.max, spec.name)
	}
	return v, nil
}

func rangeMask(lo, hi int) uint64 {
	var mask uint64
	for v := lo; v <= hi; v++ {
		mask |= 1 << uint(v)
	}
	return mask
}

func bitSet(mask uint64, v int) bool { return mask&(1<<uint(v)) != 0 }

// Next returns the first time strictly after the given time that matches the
// expression, in the time's own location. It returns the zero time if no match
// is found within roughly five years (which indicates an impossible schedule
// such as February 30th).
func (c *CronExpression) Next(after time.Time) time.Time {
	// Start from the next whole second after the reference time.
	t := after.Truncate(time.Second).Add(time.Second)
	limit := t.AddDate(5, 0, 0)

	for t.Before(limit) {
		if !bitSet(c.months, int(t.Month())) {
			// Jump to the first day of the next month.
			t = startOfNextMonth(t)
			continue
		}
		if !c.dayMatches(t) {
			t = startOfNextDay(t)
			continue
		}
		if !bitSet(c.hours, t.Hour()) {
			t = startOfNextHour(t)
			continue
		}
		if !bitSet(c.minutes, t.Minute()) {
			t = startOfNextMinute(t)
			continue
		}
		if !bitSet(c.seconds, t.Second()) {
			t = t.Add(time.Second)
			continue
		}
		return t
	}
	return time.Time{}
}

// dayMatches applies the Unix OR/AND day semantics described on CronExpression.
func (c *CronExpression) dayMatches(t time.Time) bool {
	dom := bitSet(c.daysOfMon, t.Day())
	dow := bitSet(c.daysOfWeek, int(t.Weekday()))
	switch {
	case c.domRestricted && c.dowRestricted:
		return dom || dow
	case c.domRestricted:
		return dom
	case c.dowRestricted:
		return dow
	default:
		return true
	}
}

func startOfNextMinute(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location()).Add(time.Minute)
}

func startOfNextHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).Add(time.Hour)
}

func startOfNextDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
}

func startOfNextMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
}
