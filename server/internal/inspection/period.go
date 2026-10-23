package inspection

import (
	"regexp"
	"strconv"
	"strings"
)

var coursePeriodPattern = regexp.MustCompile(`^(?:第)?(10|[1-9])(?:[-—－](10|[1-9]))?(?:节)?$`)

// ParseCoursePeriod accepts the structured form value and the legacy import
// template notation without rewriting a historical period snapshot.
func ParseCoursePeriod(value string) (first, last int, ok bool) {
	parts := coursePeriodPattern.FindStringSubmatch(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	if parts == nil {
		return 0, 0, false
	}
	first, _ = strconv.Atoi(parts[1])
	last = first
	if parts[2] != "" {
		last, _ = strconv.Atoi(parts[2])
	}
	return first, last, first <= last
}

// AttendanceHoursForPeriod applies the confirmed one section to one actual hour rule.
func AttendanceHoursForPeriod(value string) (float64, bool) {
	first, last, ok := ParseCoursePeriod(value)
	if !ok {
		return 0, false
	}
	return float64(last - first + 1), true
}
