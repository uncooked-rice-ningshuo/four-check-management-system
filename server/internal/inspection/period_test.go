package inspection

import "testing"

func TestParseCoursePeriod(t *testing.T) {
	for _, test := range []struct {
		input       string
		first, last int
		valid       bool
	}{
		{"1-2", 1, 2, true}, {"第1-2节", 1, 2, true}, {"第 3—4 节", 3, 4, true},
		{"1-1", 1, 1, true}, {"第10节", 10, 10, true},
		{"3-1", 3, 1, false}, {"0-2", 0, 0, false}, {"1-11", 0, 0, false}, {"任意", 0, 0, false},
	} {
		first, last, ok := ParseCoursePeriod(test.input)
		if first != test.first || last != test.last || ok != test.valid {
			t.Fatalf("%q: got %d-%d %v", test.input, first, last, ok)
		}
	}
}

func TestAttendanceHoursForPeriod(t *testing.T) {
	for _, test := range []struct {
		period string
		hours  float64
		ok     bool
	}{
		{"1", 1, true}, {"1-2", 2, true}, {"第3—4节", 2, true},
		{"第10节", 1, true}, {"3-1", 0, false}, {"任意", 0, false},
	} {
		hours, ok := AttendanceHoursForPeriod(test.period)
		if hours != test.hours || ok != test.ok {
			t.Fatalf("%q: got %v %v, want %v %v", test.period, hours, ok, test.hours, test.ok)
		}
	}
}
