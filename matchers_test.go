package quartz

import (
	"reflect"
	"testing"
)

func TestMatchers(t *testing.T) {
	k := NewKeyInGroup("nightly-report", "reports")
	cases := []struct {
		name    string
		matcher Matcher
		want    bool
	}{
		{"KeyEquals hit", KeyEquals(NewKeyInGroup("nightly-report", "reports")), true},
		{"KeyEquals miss group", KeyEquals(NewKey("nightly-report")), false},
		{"NameEquals hit", NameEquals("nightly-report"), true},
		{"NameEquals miss", NameEquals("weekly"), false},
		{"NameStartsWith hit", NameStartsWith("nightly"), true},
		{"NameStartsWith miss", NameStartsWith("weekly"), false},
		{"NameEndsWith hit", NameEndsWith("report"), true},
		{"NameEndsWith miss", NameEndsWith("job"), false},
		{"NameContains hit", NameContains("ly-re"), true},
		{"NameContains miss", NameContains("zzz"), false},
		{"GroupEquals hit", GroupEquals("reports"), true},
		{"GroupEquals miss", GroupEquals("jobs"), false},
		{"GroupStartsWith hit", GroupStartsWith("rep"), true},
		{"GroupEndsWith hit", GroupEndsWith("orts"), true},
		{"GroupContains hit", GroupContains("epor"), true},
		{"Anything", AnythingMatcher(), true},
		{"And hit", AndMatcher(NameStartsWith("nightly"), GroupEquals("reports")), true},
		{"And miss", AndMatcher(NameStartsWith("nightly"), GroupEquals("jobs")), false},
		{"And empty", AndMatcher(), true},
		{"Or hit", OrMatcher(GroupEquals("jobs"), GroupEquals("reports")), true},
		{"Or miss", OrMatcher(GroupEquals("jobs"), GroupEquals("misc")), false},
		{"Or empty", OrMatcher(), false},
		{"Not hit", NotMatcher(GroupEquals("jobs")), true},
		{"Not miss", NotMatcher(GroupEquals("reports")), false},
		{"Not nil", NotMatcher(nil), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.matcher.Matches(k); got != c.want {
				t.Errorf("Matches(%v) = %v, want %v", k, got, c.want)
			}
		})
	}
}

func TestMatchFuncAdapter(t *testing.T) {
	var m Matcher = MatchFunc(func(k Key) bool { return k.Name == "x" })
	if !m.Matches(NewKey("x")) || m.Matches(NewKey("y")) {
		t.Errorf("MatchFunc adapter mismatch")
	}
}

func TestMatchKeys(t *testing.T) {
	keys := []Key{
		NewKeyInGroup("a", "g1"),
		NewKeyInGroup("b", "g2"),
		NewKeyInGroup("c", "g1"),
	}
	got := MatchKeys(GroupEquals("g1"), keys)
	want := []Key{NewKeyInGroup("a", "g1"), NewKeyInGroup("c", "g1")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MatchKeys = %v, want %v", got, want)
	}
	if all := MatchKeys(nil, keys); len(all) != 3 {
		t.Errorf("nil matcher returned %d keys, want 3", len(all))
	}
}

func BenchmarkMatchKeys(b *testing.B) {
	keys := make([]Key, 100)
	for i := range keys {
		if i%2 == 0 {
			keys[i] = NewKeyInGroup("k", "even")
		} else {
			keys[i] = NewKeyInGroup("k", "odd")
		}
	}
	m := GroupEquals("even")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = MatchKeys(m, keys)
	}
}
