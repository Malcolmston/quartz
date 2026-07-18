package quartz

import "strings"

// This file ports the Matcher API from the original Quartz project. Matchers
// select jobs and triggers by their Key, and are used to filter the keys
// returned by a JobStore or Scheduler.

// Matcher reports whether a Key satisfies some criterion. Implementations must
// be safe for concurrent use.
type Matcher interface {
	// Matches reports whether the given key satisfies the matcher.
	Matches(key Key) bool
}

// MatchFunc adapts an ordinary function to the Matcher interface.
type MatchFunc func(key Key) bool

// Matches implements the Matcher interface.
func (f MatchFunc) Matches(key Key) bool { return f(key) }

// KeyEquals returns a Matcher that matches exactly the given key (both name and
// group).
func KeyEquals(key Key) Matcher {
	return MatchFunc(func(k Key) bool { return k == key })
}

// NameEquals returns a Matcher that matches keys whose name equals name,
// regardless of group.
func NameEquals(name string) Matcher {
	return MatchFunc(func(k Key) bool { return k.Name == name })
}

// NameStartsWith returns a Matcher that matches keys whose name begins with
// prefix.
func NameStartsWith(prefix string) Matcher {
	return MatchFunc(func(k Key) bool { return strings.HasPrefix(k.Name, prefix) })
}

// NameEndsWith returns a Matcher that matches keys whose name ends with suffix.
func NameEndsWith(suffix string) Matcher {
	return MatchFunc(func(k Key) bool { return strings.HasSuffix(k.Name, suffix) })
}

// NameContains returns a Matcher that matches keys whose name contains sub.
func NameContains(sub string) Matcher {
	return MatchFunc(func(k Key) bool { return strings.Contains(k.Name, sub) })
}

// GroupEquals returns a Matcher that matches keys whose group equals group.
func GroupEquals(group string) Matcher {
	return MatchFunc(func(k Key) bool { return k.Group == group })
}

// GroupStartsWith returns a Matcher that matches keys whose group begins with
// prefix.
func GroupStartsWith(prefix string) Matcher {
	return MatchFunc(func(k Key) bool { return strings.HasPrefix(k.Group, prefix) })
}

// GroupEndsWith returns a Matcher that matches keys whose group ends with
// suffix.
func GroupEndsWith(suffix string) Matcher {
	return MatchFunc(func(k Key) bool { return strings.HasSuffix(k.Group, suffix) })
}

// GroupContains returns a Matcher that matches keys whose group contains sub.
func GroupContains(sub string) Matcher {
	return MatchFunc(func(k Key) bool { return strings.Contains(k.Group, sub) })
}

// AnythingMatcher returns a Matcher that matches every key.
func AnythingMatcher() Matcher {
	return MatchFunc(func(Key) bool { return true })
}

// AndMatcher returns a Matcher that matches a key only when every supplied
// matcher matches it. With no matchers it matches everything.
func AndMatcher(matchers ...Matcher) Matcher {
	return MatchFunc(func(k Key) bool {
		for _, m := range matchers {
			if m == nil || !m.Matches(k) {
				return false
			}
		}
		return true
	})
}

// OrMatcher returns a Matcher that matches a key when at least one supplied
// matcher matches it. With no matchers it matches nothing.
func OrMatcher(matchers ...Matcher) Matcher {
	return MatchFunc(func(k Key) bool {
		for _, m := range matchers {
			if m != nil && m.Matches(k) {
				return true
			}
		}
		return false
	})
}

// NotMatcher returns a Matcher that inverts the result of the given matcher. A
// nil matcher is treated as matching everything, so its negation matches
// nothing.
func NotMatcher(m Matcher) Matcher {
	return MatchFunc(func(k Key) bool {
		if m == nil {
			return false
		}
		return !m.Matches(k)
	})
}

// MatchKeys returns the subset of keys that satisfy the matcher, preserving
// their input order. A nil matcher returns all keys.
func MatchKeys(m Matcher, keys []Key) []Key {
	out := make([]Key, 0, len(keys))
	for _, k := range keys {
		if m == nil || m.Matches(k) {
			out = append(out, k)
		}
	}
	return out
}
