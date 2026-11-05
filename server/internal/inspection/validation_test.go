package inspection

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const detailID = "abcdefab-0000-4000-8000-000000000001"

func detailContext() DetailContext {
	return DetailContext{SemesterID: detailID, WeekID: detailID, InspectionTypeID: detailID, WeekStart: "2026-09-01", WeekEnd: "2026-09-07"}
}
func validDetails(code string) map[string]json.RawMessage {
	d, _ := LookupInspectionDefinition(code)
	values := map[string]json.RawMessage{}
	for _, f := range d.Fields {
		var value string
		switch f.Kind {
		case "uuid":
			value = `"` + detailID + `"`
		case "uuid[]":
			value = `["` + detailID + `"]`
		case "date":
			value = `"2026-09-02"`
		case "time":
			value = `"08:00"`
		case "number":
			value = `2`
		case "integer":
			value = `1`
		default:
			value = `"text"`
		}
		if f.Key == "result" {
			value = `"QUALIFIED"`
		}
		if code == InspectionTypeAttendance && f.Key == "period" {
			value = `"1-2"`
		}
		if code == InspectionTypeEveningSelfStudy && f.Key == "period" {
			value = `"9-10"`
		}
		if f.Key == "round" {
			continue
		}
		values[f.Key] = json.RawMessage(value)
	}
	return values
}
func validateFixture(code string, fields map[string]json.RawMessage) (ValidatedDetails, []DetailIssue) {
	raw, _ := json.Marshal(fields)
	return ValidateDetails(code, detailContext(), raw)
}
func TestValidateInspectionDetails(t *testing.T) {
	catalog, _ := InspectionCatalog()
	for _, d := range catalog {
		t.Run(d.Code, func(t *testing.T) {
			result, issues := validateFixture(d.Code, validDetails(d.Code))
			if len(issues) != 0 || result.Details == nil || len(result.DuplicateKey) != 64 {
				t.Fatalf("valid input rejected: %+v", issues)
			}
			for _, f := range d.Fields {
				if f.Required {
					t.Run("missing_"+f.Key, func(t *testing.T) {
						fields := validDetails(d.Code)
						delete(fields, f.Key)
						_, issues := validateFixture(d.Code, fields)
						requireIssue(t, issues, "details."+f.Key, "REQUIRED")
					})
				}
				if f.MaxLength > 0 {
					fields := validDetails(d.Code)
					b, _ := json.Marshal(strings.Repeat("字", f.MaxLength+1))
					fields[f.Key] = b
					_, issues := validateFixture(d.Code, fields)
					requireIssue(t, issues, "details."+f.Key, "OUT_OF_RANGE")
				}
			}
			fields := validDetails(d.Code)
			fields["private-input-name"] = json.RawMessage(`"private-input-value"`)
			_, issues = validateFixture(d.Code, fields)
			requireIssue(t, issues, "details", "UNKNOWN_FIELD")
			encoded, _ := json.Marshal(issues)
			if strings.Contains(string(encoded), "private-input") {
				t.Fatal("unknown input leaked")
			}
			fields = validDetails(d.Code)
			fields["remark"] = json.RawMessage(`" =SUM(A1:A2) "`)
			result, issues = validateFixture(d.Code, fields)
			if len(issues) != 0 {
				t.Fatal(issues)
			}
			encoded, _ = json.Marshal(result.Details)
			if !strings.Contains(string(encoded), "=SUM(A1:A2)") {
				t.Fatal("formula text must be preserved until export")
			}
			fields = validDetails(d.Code)
			fields["date"] = json.RawMessage(`"2026-09-08"`)
			_, issues = validateFixture(d.Code, fields)
			requireIssue(t, issues, "details.date", "OUTSIDE_WEEK")
			fields["date"] = json.RawMessage(`"2026-02-30"`)
			_, issues = validateFixture(d.Code, fields)
			requireIssue(t, issues, "details.date", "INVALID_FORMAT")
		})
	}
}
func TestHygieneDescriptionIsFreeTextAndRequiredForUnqualified(t *testing.T) {
	for _, code := range []string{InspectionTypeDormitoryHygiene, InspectionTypeClassroomHygiene, InspectionTypePublicDormitoryHygiene} {
		t.Run(code, func(t *testing.T) {
			fields := validDetails(code)
			fields["result"] = json.RawMessage(`"UNQUALIFIED"`)
			delete(fields, "description")
			_, issues := validateFixture(code, fields)
			requireIssue(t, issues, "details.description", "REQUIRED")
			fields["description"] = json.RawMessage(`"来自字典的短句；录入后可以继续自定义"`)
			if _, issues := validateFixture(code, fields); len(issues) != 0 {
				t.Fatalf("custom situation text rejected: %+v", issues)
			}
		})
	}
}

func TestEveningSelfStudyOneIncidentWithoutHours(t *testing.T) {
	fields := validDetails(InspectionTypeEveningSelfStudy)
	delete(fields, "absenceHours")
	result, issues := validateFixture(InspectionTypeEveningSelfStudy, fields)
	if len(issues) != 0 || result.ViolationCount != 1 {
		t.Fatalf("expected one incident without hours, got count=%d issues=%+v", result.ViolationCount, issues)
	}
}

func TestEveningSelfStudyUsesFixedNinthAndTenthPeriods(t *testing.T) {
	fields := validDetails(InspectionTypeEveningSelfStudy)
	if _, issues := validateFixture(InspectionTypeEveningSelfStudy, fields); len(issues) != 0 {
		t.Fatalf("fixed evening rule rejected: %+v", issues)
	}
	for _, test := range []struct {
		field, value string
		path, code   string
	}{
		{"period", `"8-9"`, "details.period", "INVALID_ENUM"},
		{"period", `"9-11"`, "details.period", "INVALID_ENUM"},
	} {
		t.Run(test.field+test.value, func(t *testing.T) {
			invalid := validDetails(InspectionTypeEveningSelfStudy)
			invalid[test.field] = json.RawMessage(test.value)
			_, issues := validateFixture(InspectionTypeEveningSelfStudy, invalid)
			requireIssue(t, issues, test.path, test.code)
		})
	}
	fields["courseId"] = json.RawMessage(`"` + detailID + `"`)
	_, issues := validateFixture(InspectionTypeEveningSelfStudy, fields)
	requireIssue(t, issues, "details", "UNKNOWN_FIELD")
}

func requireIssue(t *testing.T, issues []DetailIssue, path, code string) {
	t.Helper()
	for _, i := range issues {
		if i.Path == path && i.Code == code {
			return
		}
	}
	t.Fatalf("missing %s/%s: %+v", path, code, issues)
}
func TestValidateHygieneDetails(t *testing.T) {
	for _, code := range []string{InspectionTypeDormitoryHygiene, InspectionTypeClassroomHygiene, InspectionTypePublicDormitoryHygiene} {
		t.Run(code, func(t *testing.T) {
			for _, value := range []string{`"UNKNOWN"`, `["EXCELLENT","QUALIFIED"]`, `null`} {
				f := validDetails(code)
				f["result"] = json.RawMessage(value)
				_, issues := validateFixture(code, f)
				if len(issues) == 0 {
					t.Fatal("invalid result accepted")
				}
			}
			f := validDetails(code)
			f["result"] = json.RawMessage(`"UNQUALIFIED"`)
			delete(f, "description")
			_, issues := validateFixture(code, f)
			requireIssue(t, issues, "details.description", "REQUIRED")
			f["description"] = json.RawMessage(`"可修改的自定义说明"`)
			r, issues := validateFixture(code, f)
			if len(issues) != 0 || r.OccurrenceKind != OccurrenceDate || r.ViolationCount != 1 {
				t.Fatalf("date-only free text rejected: %+v %+v", r, issues)
			}
			f = validDetails(code)
			f["responsibleClassIds"] = json.RawMessage(`[]`)
			_, issues = validateFixture(code, f)
			requireIssue(t, issues, "details.responsibleClassIds", "REQUIRED")
			f["responsibleClassIds"] = json.RawMessage(`["` + detailID + `","abcdefab-0000-4000-8000-000000000002"]`)
			_, issues = validateFixture(code, f)
			requireIssue(t, issues, "details.responsibleClassIds", "OUT_OF_RANGE")
		})
	}
	dorm, _ := validateFixture(InspectionTypeDormitoryHygiene, validDetails(InspectionTypeDormitoryHygiene))
	pub, _ := validateFixture(InspectionTypePublicDormitoryHygiene, validDetails(InspectionTypePublicDormitoryHygiene))
	if _, ok := dorm.Details.(*DormitoryHygieneDetails); !ok {
		t.Fatal("wrong dorm model")
	}
	if _, ok := pub.Details.(*PublicDormitoryHygieneDetails); !ok {
		t.Fatal("wrong public model")
	}
}

func TestDailyInspectionAdjustmentRules(t *testing.T) {
	t.Run("attendance duplicate ignores course and clock within same period", func(t *testing.T) {
		first := AttendanceInput{SemesterID: detailID, WeekID: detailID, InspectionTypeID: detailID, ClassID: detailID, StudentID: detailID, CourseID: detailID, OccurredAt: time.Date(2026, 9, 2, 8, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600)), PeriodText: "第1—2节"}
		second := first
		second.CourseID = "abcdefab-0000-4000-8000-000000000002"
		second.OccurredAt = time.Date(2026, 9, 2, 8, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
		second.PeriodText = "1-2"
		if attendanceDedupKey(first) != attendanceDedupKey(second) {
			t.Fatal("course and clock time must not split an attendance section duplicate")
		}
		second.ClassID = "abcdefab-0000-4000-8000-000000000002"
		if attendanceDedupKey(first) == attendanceDedupKey(second) {
			t.Fatal("class must remain part of attendance identity")
		}
	})
	for _, code := range []string{InspectionTypeLateReturn, InspectionTypeEveningSelfStudy} {
		t.Run(code+" date only", func(t *testing.T) {
			fields := validDetails(code)
			delete(fields, "time")
			first, issues := validateFixture(code, fields)
			if len(issues) != 0 || first.OccurrenceKind != OccurrenceDate || !first.OccurredAt.IsZero() {
				t.Fatalf("date-only event rejected: %+v %+v", first, issues)
			}
			fields["date"] = json.RawMessage(`"2026-09-03"`)
			second, issues := validateFixture(code, fields)
			if len(issues) != 0 || first.DuplicateKey == second.DuplicateKey {
				t.Fatalf("date must split event identity: %+v", issues)
			}
		})
	}
	for _, code := range []string{InspectionTypeDormitoryHygiene, InspectionTypeClassroomHygiene, InspectionTypePublicDormitoryHygiene} {
		t.Run(code+" single responsibility date only", func(t *testing.T) {
			fields := validDetails(code)
			delete(fields, "time")
			delete(fields, "round")
			delete(fields, "reasonCode")
			delete(fields, "areaId")
			result, issues := validateFixture(code, fields)
			if len(issues) != 0 || result.OccurrenceKind != OccurrenceDate {
				t.Fatalf("date-only hygiene rejected: %+v %+v", result, issues)
			}
			fields["result"] = json.RawMessage(`"UNQUALIFIED"`)
			delete(fields, "description")
			_, issues = validateFixture(code, fields)
			requireIssue(t, issues, "details.description", "REQUIRED")
			fields["description"] = json.RawMessage(`"地面和桌面需要清理"`)
			if _, issues = validateFixture(code, fields); len(issues) != 0 {
				t.Fatalf("free text situation rejected: %+v", issues)
			}
			fields["responsibleStudentIds"] = json.RawMessage(`[]`)
			_, issues = validateFixture(code, fields)
			requireIssue(t, issues, "details.responsibleStudentIds", "REQUIRED")
			fields["responsibleStudentIds"] = json.RawMessage(`[` + `"` + detailID + `","abcdefab-0000-4000-8000-000000000002"]`)
			_, issues = validateFixture(code, fields)
			requireIssue(t, issues, "details.responsibleStudentIds", "OUT_OF_RANGE")
		})
	}
}

func TestValidateDetailsRejectsRetiredHygieneFields(t *testing.T) {
	for _, code := range []string{InspectionTypeDormitoryHygiene, InspectionTypeClassroomHygiene, InspectionTypePublicDormitoryHygiene} {
		for _, field := range []string{"time", "round", "reasonCode", "areaId"} {
			t.Run(code+"/"+field, func(t *testing.T) {
				fields := validDetails(code)
				fields[field] = json.RawMessage(`"legacy"`)
				_, issues := validateFixture(code, fields)
				requireIssue(t, issues, "details", "UNKNOWN_FIELD")
			})
		}
	}
}
func TestValidateDetailsStrictInput(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{} {}`, `{"date":"2026-09-02","date":"2026-09-03"}`, `{"remark":null}`} {
		_, issues := ValidateDetails("MORNING_ATTENDANCE", detailContext(), []byte(raw))
		if len(issues) == 0 {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, value := range []string{`0`, `-1`, `24.01`, `"2"`, `1e999`} {
		f := validDetails("ATTENDANCE_RATE")
		f["absenceHours"] = json.RawMessage(value)
		_, issues := validateFixture("ATTENDANCE_RATE", f)
		if len(issues) == 0 {
			t.Fatalf("accepted %s", value)
		}
	}
	for _, value := range []string{`0`, `2`, `1.5`, `null`} {
		f := validDetails("MORNING_ATTENDANCE")
		f["absenceCount"] = json.RawMessage(value)
		_, issues := validateFixture("MORNING_ATTENDANCE", f)
		if len(issues) == 0 {
			t.Fatal("invalid count")
		}
	}
	f := validDetails("MORNING_ATTENDANCE")
	delete(f, "absenceCount")
	r, issues := validateFixture("MORNING_ATTENDANCE", f)
	if len(issues) != 0 || r.Details.(*MorningAttendanceDetails).AbsenceCount != 1 || r.OccurrenceKind != "DATE" {
		t.Fatal("default daily count")
	}
	_, issues = ValidateDetails("UNKNOWN", detailContext(), []byte(`{}`))
	requireIssue(t, issues, "inspectionType", "UNKNOWN_TYPE")
	ctx := detailContext()
	ctx.WeekEnd = "2026-08-31"
	raw, _ := json.Marshal(validDetails("LATE_RETURN"))
	_, issues = ValidateDetails("LATE_RETURN", ctx, raw)
	requireIssue(t, issues, "context.week", "INVALID_RANGE")
	f = validDetails("LATE_RETURN")
	f["studentId"] = json.RawMessage(`"00000000-0000-0000-0000-000000000000"`)
	_, issues = validateFixture("LATE_RETURN", f)
	requireIssue(t, issues, "details.studentId", "INVALID_FORMAT")
}
func TestDuplicateKey(t *testing.T) {
	catalog, _ := InspectionCatalog()
	for _, d := range catalog {
		t.Run(d.Code, func(t *testing.T) {
			f := validDetails(d.Code)
			a, _ := validateFixture(d.Code, f)
			f["remark"] = json.RawMessage(`"changed"`)
			b, _ := validateFixture(d.Code, f)
			if a.DuplicateKey != b.DuplicateKey {
				t.Fatal("remark changed identity")
			}
			f["date"] = json.RawMessage(`"2026-09-03"`)
			b, _ = validateFixture(d.Code, f)
			if a.DuplicateKey == b.DuplicateKey {
				t.Fatal("date absent from identity")
			}
			f = validDetails(d.Code)
			for key, value := range f {
				f[key] = json.RawMessage(strings.ReplaceAll(string(value), detailID, strings.ToUpper(detailID)))
			}
			b, issues := validateFixture(d.Code, f)
			if len(issues) != 0 || a.DuplicateKey != b.DuplicateKey {
				t.Fatal("UUID case changes identity")
			}
		})
	}
	f := validDetails(InspectionTypeAttendance)
	f["period"] = json.RawMessage(`" 1-2 "`)
	a, _ := validateFixture(InspectionTypeAttendance, f)
	expected := AttendanceDedupKey(AttendanceRecord{SemesterID: detailID, WeekID: detailID, InspectionTypeID: detailID, ClassID: detailID, StudentID: detailID, CourseID: detailID, OccurredAt: time.Date(2026, 9, 2, 8, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600)), PeriodText: "1-2"})
	if a.DuplicateKey != expected {
		t.Fatal("attendance key does not use normalized class/student/date/period")
	}
	f = validDetails(InspectionTypeDormitoryHygiene)
	a, _ = validateFixture(InspectionTypeDormitoryHygiene, f)
	f["result"] = json.RawMessage(`"UNQUALIFIED"`)
	f["description"] = json.RawMessage(`"发现问题"`)
	b, issues := validateFixture(InspectionTypeDormitoryHygiene, f)
	if len(issues) != 0 || a.DuplicateKey != b.DuplicateKey || b.ViolationCount != 1 {
		t.Fatal("result and description must not split same-day identity")
	}
}

func TestValidateDetailsBoundaries(t *testing.T) {
	catalog, _ := InspectionCatalog()
	for _, d := range catalog {
		t.Run(d.Code, func(t *testing.T) {
			for _, date := range []string{"2026-09-01", "2026-09-07"} {
				f := validDetails(d.Code)
				f["date"], _ = json.Marshal(date)
				if _, issues := validateFixture(d.Code, f); len(issues) != 0 {
					t.Fatal(issues)
				}
			}
			for _, field := range d.Fields {
				if field.MaxLength > 0 && field.Key != "round" && !(d.Code == InspectionTypeAttendance && field.Key == "period") {
					f := validDetails(d.Code)
					f[field.Key], _ = json.Marshal(strings.Repeat("字", field.MaxLength))
					if _, issues := validateFixture(d.Code, f); len(issues) != 0 {
						t.Fatalf("max %s: %+v", field.Key, issues)
					}
				}
				if field.Kind == "uuid" {
					f := validDetails(d.Code)
					f[field.Key] = json.RawMessage(`"not-a-uuid"`)
					_, issues := validateFixture(d.Code, f)
					requireIssue(t, issues, "details."+field.Key, "INVALID_FORMAT")
				}
			}
		})
	}
	f := validDetails(InspectionTypeLateReturn)
	f["time"] = json.RawMessage(`"23:59"`)
	_, issues := validateFixture(InspectionTypeLateReturn, f)
	requireIssue(t, issues, "details", "UNKNOWN_FIELD")
	f = validDetails(InspectionTypeAttendance)
	f["absenceHours"] = json.RawMessage(`24`)
	_, issues = validateFixture(InspectionTypeAttendance, f)
	requireIssue(t, issues, "details.absenceHours", "PERIOD_HOURS_MISMATCH")
	f["period"] = json.RawMessage(`"1\u001f2"`)
	_, issues = validateFixture(InspectionTypeAttendance, f)
	requireIssue(t, issues, "details.period", "INVALID_FORMAT")
	f = validDetails(InspectionTypeLateReturn)
	f["remark"] = json.RawMessage(`"safe\u0000text"`)
	_, issues = validateFixture(InspectionTypeLateReturn, f)
	requireIssue(t, issues, "details.remark", "INVALID_FORMAT")
}

func TestDuplicateKeyComponents(t *testing.T) {
	catalog, _ := InspectionCatalog()
	alternateID := "abcdefab-0000-4000-8000-000000000002"
	for _, d := range catalog {
		t.Run(d.Code, func(t *testing.T) {
			original, _ := validateFixture(d.Code, validDetails(d.Code))
			for _, component := range d.DuplicateKey {
				f := validDetails(d.Code)
				ctx := detailContext()
				switch component {
				case "semesterId":
					ctx.SemesterID = alternateID
				case "weekId":
					ctx.WeekID = alternateID
				case "inspectionTypeId":
					ctx.InspectionTypeID = alternateID
				case "date":
					f["date"] = json.RawMessage(`"2026-09-03"`)
				case "period":
					f["period"] = json.RawMessage(`"3-4"`)
				case "occurredAt":
					f["time"] = json.RawMessage(`"08:01"`)
				case "responsibleClassIds", "responsibleStudentIds", "responsibleDormitoryIds":
					f[component], _ = json.Marshal([]string{alternateID})
				default:
					f[component], _ = json.Marshal(alternateID)
				}
				raw, _ := json.Marshal(f)
				changed, issues := ValidateDetails(d.Code, ctx, raw)
				if len(issues) != 0 || changed.DuplicateKey == original.DuplicateKey {
					t.Fatalf("identity component %s missing: %+v", component, issues)
				}
			}
		})
	}
}

func TestValidateDetailsStoragePrecision(t *testing.T) {
	fields := validDetails(InspectionTypeAttendance)
	fields["absenceHours"] = json.RawMessage(`0.001`)
	_, issues := validateFixture(InspectionTypeAttendance, fields)
	requireIssue(t, issues, "details.absenceHours", "OUT_OF_RANGE")
}
