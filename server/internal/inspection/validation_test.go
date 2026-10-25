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
func TestValidateSevenDetails(t *testing.T) {
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
func TestHygieneOtherReasonRequiresDescription(t *testing.T) {
	for _, code := range []string{InspectionTypeDormitoryHygiene, InspectionTypeClassroomHygiene, InspectionTypePublicDormitoryHygiene} {
		t.Run(code, func(t *testing.T) {
			fields := validDetails(code)
			fields["result"] = json.RawMessage(`"UNQUALIFIED"`)
			fields["reasonCode"] = json.RawMessage(`"OTHER"`)
			for _, description := range []string{`""`, `"  "`} {
				fields["description"] = json.RawMessage(description)
				_, issues := validateFixture(code, fields)
				requireIssue(t, issues, "details.description", "OTHER_REASON_REQUIRED")
			}
			fields["description"] = json.RawMessage(`"门框积灰"`)
			if _, issues := validateFixture(code, fields); len(issues) != 0 {
				t.Fatalf("custom reason rejected: %+v", issues)
			}
		})
	}
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
	for _, code := range []string{"DORMITORY_HYGIENE", "CLASSROOM_HYGIENE", "PUBLIC_DORMITORY_HYGIENE"} {
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
			delete(f, "reasonCode")
			_, issues := validateFixture(code, f)
			requireIssue(t, issues, "details.reasonCode", "REQUIRED")
			f = validDetails(code)
			delete(f, "time")
			_, issues = validateFixture(code, f)
			requireIssue(t, issues, "details.round", "OCCURRENCE_REQUIRED")
			f["round"] = json.RawMessage(`" first "`)
			r, issues := validateFixture(code, f)
			if len(issues) != 0 || r.OccurrenceKind != "ROUND" || !r.OccurredAt.IsZero() || r.Round != "first" || r.ViolationCount != 0 {
				t.Fatalf("bad round: %+v %+v", r, issues)
			}
			f["time"] = json.RawMessage(`"08:00"`)
			_, issues = validateFixture(code, f)
			requireIssue(t, issues, "details.round", "OCCURRENCE_CONFLICT")
			f = validDetails(code)
			f["responsibleClassIds"] = json.RawMessage(`[]`)
			_, issues = validateFixture(code, f)
			requireIssue(t, issues, "details.responsibleClassIds", "REQUIRED")
			f["responsibleClassIds"] = json.RawMessage(`["` + detailID + `","` + strings.ToUpper(detailID) + `"]`)
			_, issues = validateFixture(code, f)
			requireIssue(t, issues, "details.responsibleClassIds[1]", "DUPLICATE_ITEM")
		})
	}
	dorm, _ := validateFixture("DORMITORY_HYGIENE", validDetails("DORMITORY_HYGIENE"))
	public, _ := validateFixture("PUBLIC_DORMITORY_HYGIENE", validDetails("PUBLIC_DORMITORY_HYGIENE"))
	if _, ok := dorm.Details.(*DormitoryHygieneDetails); !ok {
		t.Fatal("wrong dorm model")
	}
	if _, ok := public.Details.(*PublicDormitoryHygieneDetails); !ok {
		t.Fatal("wrong public model")
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
	f := validDetails("ATTENDANCE_RATE")
	f["period"] = json.RawMessage(`" 1-2 "`)
	a, _ := validateFixture("ATTENDANCE_RATE", f)
	expected := AttendanceDedupKey(AttendanceRecord{SemesterID: detailID, WeekID: detailID, InspectionTypeID: detailID, StudentID: detailID, CourseID: detailID, OccurredAt: time.Date(2026, 9, 2, 8, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600)), PeriodText: "1-2"})
	if a.DuplicateKey != expected {
		t.Fatal("legacy attendance hash changed")
	}
	f = validDetails("DORMITORY_HYGIENE")
	a, _ = validateFixture("DORMITORY_HYGIENE", f)
	f["result"] = json.RawMessage(`"UNQUALIFIED"`)
	b, issues := validateFixture("DORMITORY_HYGIENE", f)
	if len(issues) != 0 || a.DuplicateKey != b.DuplicateKey || b.ViolationCount != 1 {
		t.Fatal("result must use versioned edit")
	}
	delete(f, "time")
	f["round"] = json.RawMessage(`"08:00"`)
	b, _ = validateFixture("DORMITORY_HYGIENE", f)
	if a.DuplicateKey == b.DuplicateKey {
		t.Fatal("time and round collision")
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
	for _, clock := range []string{"24:00", "08:60", "8:00", "08:00:00.5", "08:00Z"} {
		f := validDetails("LATE_RETURN")
		f["time"], _ = json.Marshal(clock)
		_, issues := validateFixture("LATE_RETURN", f)
		requireIssue(t, issues, "details.time", "INVALID_FORMAT")
	}
	for _, clock := range []string{"00:00", "23:59:59"} {
		f := validDetails("LATE_RETURN")
		f["time"], _ = json.Marshal(clock)
		if _, issues := validateFixture("LATE_RETURN", f); len(issues) != 0 {
			t.Fatal(issues)
		}
	}
	f := validDetails("ATTENDANCE_RATE")
	f["absenceHours"] = json.RawMessage(`24`)
	_, issues := validateFixture("ATTENDANCE_RATE", f)
	requireIssue(t, issues, "details.absenceHours", "PERIOD_HOURS_MISMATCH")
	f["period"] = json.RawMessage(`"1\u001f2"`)
	_, issues = validateFixture("ATTENDANCE_RATE", f)
	requireIssue(t, issues, "details.period", "INVALID_FORMAT")
	f = validDetails("LATE_RETURN")
	f["remark"] = json.RawMessage(`"safe\u0000text"`)
	_, issues = validateFixture("LATE_RETURN", f)
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
				case "occurredAt", "occurrenceDiscriminator":
					f["time"] = json.RawMessage(`"08:01"`)
				case "date":
					f["date"] = json.RawMessage(`"2026-09-03"`)
				case "period":
					f["period"] = json.RawMessage(`"3-4"`)
				default:
					f[component], _ = json.Marshal(alternateID)
				}
				raw, _ := json.Marshal(f)
				changed, issues := ValidateDetails(d.Code, ctx, raw)
				if len(issues) != 0 || changed.DuplicateKey == original.DuplicateKey {
					t.Fatalf("identity component %s missing: %+v", component, issues)
				}
			}
			if d.Granularity == "OBJECT_INSPECTION" {
				f := validDetails(d.Code)
				f["responsibleClassIds"] = json.RawMessage(`["` + alternateID + `"]`)
				changed, issues := validateFixture(d.Code, f)
				if len(issues) != 0 || changed.DuplicateKey != original.DuplicateKey {
					t.Fatal("responsibility changes event identity")
				}
			}
			if d.Granularity != "STUDENT_DAY" {
				f := validDetails(d.Code)
				f["time"] = json.RawMessage(`"08:00:00"`)
				changed, issues := validateFixture(d.Code, f)
				if len(issues) != 0 || changed.DuplicateKey != original.DuplicateKey {
					t.Fatal("equivalent clocks differ")
				}
			}
		})
	}
}

func TestValidateDetailsStoragePrecision(t *testing.T) {
	for _, code := range []string{InspectionTypeAttendance, InspectionTypeUnattended} {
		fields := validDetails(code)
		fields["absenceHours"] = json.RawMessage(`0.001`)
		_, issues := validateFixture(code, fields)
		requireIssue(t, issues, "details.absenceHours", "OUT_OF_RANGE")
	}
}
