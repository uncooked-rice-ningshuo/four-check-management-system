package inspection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ValidateDetails enforces the catalog's input contract without granting access,
// performing I/O, or resolving database references. Formula-like text is retained:
// spreadsheet formula escaping belongs at the output boundary, not in stored data.
func ValidateDetails(code string, context DetailContext, raw json.RawMessage) (ValidatedDetails, []DetailIssue) {
	adapter, ok := detailAdapters[code]
	if !ok {
		return ValidatedDetails{}, []DetailIssue{detailIssue("inspectionType", "UNKNOWN_TYPE")}
	}
	definition, err := LookupInspectionDefinition(code)
	if err != nil {
		return ValidatedDetails{}, []DetailIssue{detailIssue("inspectionType", "UNKNOWN_TYPE")}
	}
	values, err := decodeDetailObject(raw)
	if err != nil {
		return ValidatedDetails{}, []DetailIssue{detailIssue("details", "INVALID_OBJECT")}
	}
	issues := validateDetailContext(&context)
	known := make(map[string]bool, len(definition.Fields))
	for _, f := range definition.Fields {
		known[f.Key] = true
	}
	for key := range values {
		if !known[key] {
			issues = append(issues, detailIssue("details", "UNKNOWN_FIELD"))
			break
		}
	}
	for _, field := range definition.Fields {
		normalized, fieldIssues := normalizeDetailField(field, values[field.Key])
		issues = append(issues, fieldIssues...)
		if normalized != nil {
			values[field.Key] = normalized
		}
	}
	// No typed model or duplicate key is returned for malformed data.
	if len(issues) > 0 {
		return ValidatedDetails{}, issues
	}
	if code == InspectionTypeAttendance {
		hours, valid := AttendanceHoursForPeriod(detailString(values, "period"))
		var submitted float64
		if !valid || json.Unmarshal(values["absenceHours"], &submitted) != nil || submitted != hours {
			return ValidatedDetails{}, []DetailIssue{detailIssue("details.absenceHours", "PERIOD_HOURS_MISMATCH")}
		}
	}
	date := detailString(values, "date")
	if date < context.WeekStart || date > context.WeekEnd {
		issues = append(issues, detailIssue("details.date", "OUTSIDE_WEEK"))
	}
	clock, round := detailString(values, "time"), detailString(values, "round")
	isHygiene := definition.Granularity == "OBJECT_INSPECTION"
	if isHygiene {
		if clock == "" && round == "" {
			issues = append(issues, detailIssue("details.round", "OCCURRENCE_REQUIRED"))
		}
		if clock != "" && round != "" {
			issues = append(issues, detailIssue("details.round", "OCCURRENCE_CONFLICT"))
		}
		if detailString(values, "result") == HygieneUnqualified && detailString(values, "reasonCode") == "" {
			issues = append(issues, detailIssue("details.reasonCode", "REQUIRED"))
		}
	}
	if len(issues) > 0 {
		return ValidatedDetails{}, issues
	}
	result := ValidatedDetails{Date: date, Round: round, OccurrenceKind: OccurrenceDate, ViolationCount: 1}
	if clock != "" {
		// Input is a local school clock. UTC+08 is explicit and independent of host timezone.
		occurred, parseErr := time.Parse(time.RFC3339, date+"T"+clock+"+08:00")
		if parseErr != nil {
			return ValidatedDetails{}, []DetailIssue{detailIssue("details.time", "INVALID_FORMAT")}
		}
		result.OccurredAt = occurred
		result.OccurrenceKind = OccurrenceTime
	} else if round != "" {
		result.OccurrenceKind = OccurrenceRound
	}
	if isHygiene && detailString(values, "result") != HygieneUnqualified {
		result.ViolationCount = 0
	}
	result.Details = adapter.newDetails()
	normalized, marshalErr := json.Marshal(values)
	if marshalErr != nil || json.Unmarshal(normalized, result.Details) != nil {
		return ValidatedDetails{}, []DetailIssue{detailIssue("details", "INVALID_OBJECT")}
	}
	result.DuplicateKey = detailDuplicateKey(code, context, values, result)
	return result, nil
}

// Reject duplicate members, trailing JSON and non-objects; ordinary unmarshalling
// silently accepts duplicate keys, which makes preview and confirmation ambiguous.
func decodeDetailObject(raw []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("invalid input")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("object required")
	}
	values := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("invalid member")
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate member")
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing input")
	}
	return values, nil
}
func normalizeDetailField(f FieldDefinition, raw json.RawMessage) (json.RawMessage, []DetailIssue) {
	path := "details." + f.Key
	fail := func(code string) (json.RawMessage, []DetailIssue) { return nil, []DetailIssue{detailIssue(path, code)} }
	if len(raw) == 0 {
		if f.Required {
			return fail("REQUIRED")
		}
		if f.ConstValue != nil {
			return json.RawMessage(fmt.Sprint(*f.ConstValue)), nil
		}
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if f.Required {
			return fail("REQUIRED")
		}
		return fail("INVALID_TYPE")
	}
	switch f.Kind {
	case "uuid[]":
		var ids []string
		if json.Unmarshal(raw, &ids) != nil {
			return fail("INVALID_TYPE")
		}
		if len(ids) < f.MinItems {
			return fail("REQUIRED")
		}
		seen := map[string]bool{}
		for i, id := range ids {
			canonical, ok := canonicalDetailID(id)
			if !ok {
				return nil, []DetailIssue{detailIssue(fmt.Sprintf("%s[%d]", path, i), "INVALID_FORMAT")}
			}
			if seen[canonical] {
				return nil, []DetailIssue{detailIssue(fmt.Sprintf("%s[%d]", path, i), "DUPLICATE_ITEM")}
			}
			ids[i] = canonical
			seen[canonical] = true
		}
		sort.Strings(ids)
		normalized, _ := json.Marshal(ids)
		return normalized, nil
	case "number", "integer":
		var n float64
		if json.Unmarshal(raw, &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return fail("INVALID_TYPE")
		}
		if f.Kind == "integer" && n != math.Trunc(n) {
			return fail("INVALID_TYPE")
		}
		// attendance_details uses numeric(6,2); reject rounding instead of changing business values on save.
		if f.Key == "absenceHours" && n != math.Round(n*100)/100 {
			return fail("OUT_OF_RANGE")
		}
		if (f.Minimum != nil && (n < *f.Minimum || (f.ExclusiveMinimum && n == *f.Minimum))) || (f.Maximum != nil && n > *f.Maximum) || (f.ConstValue != nil && n != *f.ConstValue) {
			return fail("OUT_OF_RANGE")
		}
		normalized, _ := json.Marshal(n)
		return normalized, nil
	default:
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return fail("INVALID_TYPE")
		}
		value = strings.TrimSpace(value)
		if f.Required && value == "" {
			return fail("REQUIRED")
		}
		if f.MaxLength > 0 && utf8.RuneCountInString(value) > f.MaxLength {
			return fail("OUT_OF_RANGE")
		}
		// NUL is invalid in PostgreSQL text; separators/control characters cannot enter event keys.
		for _, r := range value {
			if unicode.IsControl(r) && (f.Key != "remark" && f.Key != "description" || r != '\n' && r != '\r' && r != '\t') {
				return fail("INVALID_FORMAT")
			}
		}
		if value != "" {
			switch f.Kind {
			case "uuid":
				canonical, ok := canonicalDetailID(value)
				if !ok {
					return fail("INVALID_FORMAT")
				}
				value = canonical
			case "date":
				parsed, err := time.Parse("2006-01-02", value)
				if err != nil || parsed.Format("2006-01-02") != value || parsed.Year() < 1 {
					return fail("INVALID_FORMAT")
				}
			case "time":
				parsed, err := time.Parse("15:04", value)
				if err != nil || parsed.Format("15:04") != value {
					parsed, err = time.Parse("15:04:05", value)
					if err != nil || parsed.Format("15:04:05") != value {
						return fail("INVALID_FORMAT")
					}
				}
				value = parsed.Format("15:04:05")
			}
		}
		if len(f.Enum) > 0 {
			found := false
			for _, v := range f.Enum {
				found = found || value == v
			}
			if !found {
				return fail("INVALID_ENUM")
			}
		}
		normalized, _ := json.Marshal(value)
		return normalized, nil
	}
}
func canonicalDetailID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	id, err := uuid.Parse(value)
	if err != nil || len(value) != 36 || id == uuid.Nil {
		return "", false
	}
	return id.String(), true
}
func validateDetailContext(c *DetailContext) []DetailIssue {
	var issues []DetailIssue
	for _, field := range []struct {
		path  string
		value *string
	}{{"context.semesterId", &c.SemesterID}, {"context.weekId", &c.WeekID}, {"context.inspectionTypeId", &c.InspectionTypeID}} {
		value, ok := canonicalDetailID(*field.value)
		if !ok {
			issues = append(issues, detailIssue(field.path, "INVALID_FORMAT"))
		} else {
			*field.value = value
		}
	}
	start, e1 := time.Parse("2006-01-02", c.WeekStart)
	end, e2 := time.Parse("2006-01-02", c.WeekEnd)
	if e1 != nil || e2 != nil || start.Year() < 1 || end.Year() < 1 || start.Format("2006-01-02") != c.WeekStart || end.Format("2006-01-02") != c.WeekEnd || start.After(end) {
		issues = append(issues, detailIssue("context.week", "INVALID_RANGE"))
	}
	return issues
}
func detailString(values map[string]json.RawMessage, key string) string {
	var value string
	if raw, ok := values[key]; ok {
		if json.Unmarshal(raw, &value) != nil {
			return ""
		}
	}
	return value
}
func detailDuplicateKey(code string, c DetailContext, v map[string]json.RawMessage, r ValidatedDetails) string {
	if code == InspectionTypeAttendance || code == InspectionTypeUnattended {
		return attendanceDedupKey(AttendanceInput{SemesterID: c.SemesterID, WeekID: c.WeekID, InspectionTypeID: c.InspectionTypeID, StudentID: detailString(v, "studentId"), CourseID: detailString(v, "courseId"), OccurredAt: r.OccurredAt, PeriodText: detailString(v, "period")})
	}
	parts := []string{c.SemesterID, c.WeekID, c.InspectionTypeID}
	switch code {
	case InspectionTypeMorning:
		parts = append(parts, detailString(v, "studentId"), r.Date)
	case InspectionTypeLateReturn:
		parts = append(parts, detailString(v, "studentId"), r.OccurredAt.UTC().Format(time.RFC3339Nano))
	default:
		switch code {
		case InspectionTypeDormitoryHygiene:
			parts = append(parts, detailString(v, "dormitoryId"))
		case InspectionTypeClassroomHygiene:
			parts = append(parts, detailString(v, "classroomId"), detailString(v, "areaId"))
		case InspectionTypePublicDormitoryHygiene:
			parts = append(parts, detailString(v, "areaId"))
		}
		discriminator := r.Round
		if r.OccurrenceKind == OccurrenceTime {
			discriminator = detailString(v, "time")
		}
		parts = append(parts, r.Date, r.OccurrenceKind, discriminator)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(digest[:])
}
func detailIssue(path, code string) DetailIssue {
	messages := map[string]string{
		"UNKNOWN_TYPE": "不支持的检查类型", "INVALID_OBJECT": "检查明细必须是有效且字段不重复的 JSON 对象",
		"UNKNOWN_FIELD": "检查明细包含不支持的字段", "REQUIRED": "请填写必填字段",
		"INVALID_TYPE": "字段数据类型不正确", "INVALID_FORMAT": "字段格式不正确", "OUT_OF_RANGE": "字段超出允许范围",
		"INVALID_ENUM": "请选择有效的检查结果", "DUPLICATE_ITEM": "关联列表存在重复项", "INVALID_RANGE": "周日期范围无效",
		"OUTSIDE_WEEK": "检查日期不在所选周内", "OCCURRENCE_REQUIRED": "请填写检查时间或轮次", "OCCURRENCE_CONFLICT": "检查时间和轮次只能填写一项",
	}
	return DetailIssue{Path: path, Code: code, Message: messages[code]}
}
