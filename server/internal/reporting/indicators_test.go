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

func TestCalculateReportIndicatorsSeparatesSameNamedClassesByID(t *testing.T) {
	inputs := ReportMetricInputs{WeekMultiplier: 1, ReportGradeScopeVersion: 1,
		Grades: []ReportGradeSizeSnapshot{{GradeID: "grade-a", Grade: "2026级", ExpectedStudentCount: 5}},
		Classes: []ReportClassSizeSnapshot{
			{ClassID: "class-a", GradeID: "grade-a", Grade: "2026级", ClassName: "一班", MajorName: "软件工程", ExpectedStudentCount: 2},
			{ClassID: "class-b", GradeID: "grade-a", Grade: "2026级", ClassName: "一班", MajorName: "网络工程", ExpectedStudentCount: 3},
		}}
	rows := []ReportCountRow{{InspectionTypeCode: inspection.InspectionTypeAttendance}}
	records := []Record{{TypeCode: inspection.InspectionTypeAttendance, ClassID: "class-b", Grade: "2026级", ClassName: "一班", StudentNumber: "student-b"}}
	result, err := CalculateReportIndicators(inputs, rows, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Classes) != 2 || result.Classes[0].ClassID != "class-a" || result.Classes[1].ClassID != "class-b" || result.Classes[0].AttendanceRate.ViolationStudentCount != 0 || result.Classes[1].AttendanceRate.ViolationStudentCount != 1 {
		t.Fatalf("same-named classes merged: %+v", result.Classes)
	}
}

func TestApplyReportSourceStatesDistinguishesZeroAndMissing(t *testing.T) {
	content := &ReportVersionContent{
		StatisticsRuleVersion: "WEEKLY_INDICATORS_V2",
		Rows: []ReportCountRow{
			{InspectionTypeID: "attendance", InspectionTypeCode: inspection.InspectionTypeAttendance, RecordCount: 1, DataStatus: "CONFIRMED_RECORDS"},
			{InspectionTypeID: "morning", InspectionTypeCode: inspection.InspectionTypeMorning, ZeroReceiptCount: 1, DataStatus: "CONFIRMED_ZERO_RECORD_RECEIPT"},
			{InspectionTypeID: "late", InspectionTypeCode: inspection.InspectionTypeLateReturn, DataStatus: "NO_CONFIRMED_DATA"},
		},
		Indicators: &ReportIndicators{
			IncludedTypeCodes: []string{inspection.InspectionTypeAttendance, inspection.InspectionTypeMorning, inspection.InspectionTypeLateReturn},
			Classes: []ReportClassIndicators{{ClassID: "class-a", ReportIndicatorValues: ReportIndicatorValues{
				AttendanceRate: &ReportAttendanceRateMetric{ViolationStudentCount: 1},
			}}},
		},
	}
	ApplyReportSourceStates(content)
	got := content.Indicators.Classes[0].SourceStates
	if got[inspection.InspectionTypeAttendance] != "CONFIRMED_WITH_RECORDS" || got[inspection.InspectionTypeMorning] != "CONFIRMED_ZERO" || got[inspection.InspectionTypeLateReturn] != "SOURCE_INCOMPLETE" {
		t.Fatalf("class source states = %+v", got)
	}
}
