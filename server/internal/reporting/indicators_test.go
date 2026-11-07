package reporting

import (
	"testing"

	"four-check-management-system/server/internal/inspection"
)

func TestCalculateReportIndicatorsUsesGradeRosterForWeekTotals(t *testing.T) {
	inputs := ReportMetricInputs{
		WeekMultiplier:      1,
		MorningPlannedDates: []string{"2026-03-02", "2026-03-03", "2026-03-04", "2026-03-05", "2026-03-06"},
		Grades:              []ReportGradeSizeSnapshot{{Grade: "2026级", ExpectedStudentCount: 100}},
		Classes: []ReportClassSizeSnapshot{
			{Grade: "2026级", ClassName: "一班", ExpectedStudentCount: 26},
			{Grade: "2026级", ClassName: "二班", ExpectedStudentCount: 24},
		},
	}
	rows := []ReportCountRow{
		{InspectionTypeCode: inspection.InspectionTypeAttendance},
		{InspectionTypeCode: inspection.InspectionTypeMorning},
	}
	records := []Record{
		{ID: "attendance-a", TypeCode: inspection.InspectionTypeAttendance, StudentNumber: "S-001", Grade: "2026级", ClassName: "一班"},
		{ID: "attendance-b", TypeCode: inspection.InspectionTypeAttendance, StudentNumber: "S-002", Grade: "2026级", ClassName: "二班"},
		{ID: "morning-a", TypeCode: inspection.InspectionTypeMorning, Grade: "2026级", ClassName: "一班"},
		{ID: "morning-b", TypeCode: inspection.InspectionTypeMorning, Grade: "2026级", ClassName: "二班"},
	}

	result, err := CalculateReportIndicators(inputs, rows, records)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Totals.AttendanceRate; got == nil || got.ExpectedStudentCount != 100 || got.ViolationStudentCount != 2 || got.AttendedStudentCount != 98 || got.Rate == nil || *got.Rate != 98 {
		t.Fatalf("grade-wide attendance metric = %+v, want 98 of 100", got)
	}
	if got := result.Totals.MorningAttendance; got == nil || got.ExpectedPersonTimes != 500 || got.AbsencePersonTimes != 2 || got.AttendedPersonTimes != 498 || got.Rate == nil || *got.Rate != 99.6 {
		t.Fatalf("grade-wide morning metric = %+v, want 498 of 500", got)
	}
	if len(result.Classes) != 2 || result.Classes[0].AttendanceRate == nil || result.Classes[0].AttendanceRate.ExpectedStudentCount != 26 || result.Classes[1].AttendanceRate == nil || result.Classes[1].AttendanceRate.ExpectedStudentCount != 24 {
		t.Fatalf("class attendance denominators = %+v, want 26 and 24", result.Classes)
	}
}

func TestCalculateReportIndicatorsKeepsLegacyClassDenominator(t *testing.T) {
	result, err := CalculateReportIndicators(ReportMetricInputs{
		WeekMultiplier:      1,
		MorningPlannedDates: []string{},
		Classes:             []ReportClassSizeSnapshot{{Grade: "2026级", ClassName: "一班", ExpectedStudentCount: 26}},
	}, []ReportCountRow{{InspectionTypeCode: inspection.InspectionTypeAttendance}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Totals.AttendanceRate; got == nil || got.ExpectedStudentCount != 26 || got.Rate == nil || *got.Rate != 100 {
		t.Fatalf("legacy attendance metric = %+v, want 26 of 26", got)
	}
}

func TestCalculateReportIndicatorsV2KeepsEmptyClassesAndRejectsOutsideScope(t *testing.T) {
	input := ReportMetricInputs{WeekMultiplier: 1, ReportGradeScopeVersion: 1,
		Grades: []ReportGradeSizeSnapshot{{GradeID: "grade-a", Grade: "2026级", ExpectedStudentCount: 5}},
		Classes: []ReportClassSizeSnapshot{
			{ClassID: "class-a", GradeID: "grade-a", Grade: "2026级", ClassName: "一班", ExpectedStudentCount: 5},
			{ClassID: "class-b", GradeID: "grade-a", Grade: "2026级", ClassName: "二班", ExpectedStudentCount: 0},
		},
	}
	rows := []ReportCountRow{{InspectionTypeCode: inspection.InspectionTypeAttendance}}
	result, err := CalculateReportIndicators(input, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Classes) != 2 || result.Classes[1].ClassID != "class-b" || result.Classes[1].ExpectedStudentCount != 0 {
		t.Fatalf("empty class missing from V2 snapshot: %+v", result.Classes)
	}
	_, err = CalculateReportIndicators(input, rows, []Record{{ID: "outside", TypeCode: inspection.InspectionTypeAttendance, StudentNumber: "S-1", Grade: "2025级", ClassName: "一班"}})
	if err == nil {
		t.Fatal("out-of-scope record was accepted")
	}
}
