package reporting

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"four-check-management-system/server/internal/inspection"
)

type indicatorAccumulator struct {
	classID              string
	grade                string
	className            string
	expectedStudents     int
	attendancePeople     map[string]struct{}
	unattendedPeople     map[string]struct{}
	unattendedRaw        int
	morningRaw           int
	eveningRaw           int
	classroomHygieneRaw  int
	dormitoryPersonalRaw int
	dormitoryPublicRaw   int
	lateReturnRaw        int
}

// CalculateReportIndicators calculates weekly indicators from the frozen inputs
// and the exact record set included in one report version.
func CalculateReportIndicators(inputs ReportMetricInputs, rows []ReportCountRow, records []Record) (*ReportIndicators, error) {
	if inputs.WeekMultiplier != 1 && inputs.WeekMultiplier != 2 {
		return nil, fmt.Errorf("invalid weekly report multiplier")
	}
	for _, class := range inputs.Classes {
		if strings.TrimSpace(class.ClassName) == "" || class.ExpectedStudentCount < 0 {
			return nil, fmt.Errorf("invalid weekly report class snapshot")
		}
	}
	for _, grade := range inputs.Grades {
		if strings.TrimSpace(grade.Grade) == "" || grade.ExpectedStudentCount < 0 {
			return nil, fmt.Errorf("invalid weekly report grade snapshot")
		}
	}

	included := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		code := row.InspectionTypeCode
		if code == "" {
			code = reportTypeCodeForName(row.InspectionType)
		}
		if code != "" {
			included[code] = struct{}{}
		}
	}

	byClass := make(map[string]*indicatorAccumulator, len(inputs.Classes))
	for _, class := range inputs.Classes {
		key := reportClassKey(class.Grade, class.ClassName)
		accumulator := byClass[key]
		if accumulator == nil {
			accumulator = newIndicatorAccumulator(class.Grade, class.ClassName)
			byClass[key] = accumulator
		}
		if inputs.ReportGradeScopeVersion > 0 && accumulator.classID != "" && accumulator.classID != class.ClassID {
			return nil, fmt.Errorf("%w: duplicate class name", ErrReportMetricScopeInvalid)
		}
		accumulator.classID = class.ClassID
		accumulator.expectedStudents += class.ExpectedStudentCount
	}
	gradeTotals := make(map[string]int, len(inputs.Grades))
	for _, grade := range inputs.Grades {
		gradeTotals[grade.Grade] += grade.ExpectedStudentCount
	}
	totalExpectedStudents := 0
	if len(inputs.Grades) > 0 {
		for _, count := range gradeTotals {
			totalExpectedStudents += count
		}
	} else {
		// Older immutable versions only stored class sizes; retain their original denominator.
		for _, class := range inputs.Classes {
			totalExpectedStudents += class.ExpectedStudentCount
		}
	}

	attendancePeople := make(map[string]struct{})
	unattendedPeople := make(map[string]struct{})
	for _, record := range records {
		classes := reportRecordClasses(record)
		if len(classes) == 0 {
			continue
		}
		for _, class := range classes {
			if byClass[class.key] == nil {
				if inputs.ReportGradeScopeVersion > 0 {
					return nil, fmt.Errorf("%w: record class outside scope", ErrReportMetricScopeInvalid)
				}
				byClass[class.key] = newIndicatorAccumulator(class.grade, class.className)
			}
		}
		switch record.TypeCode {
		case inspection.InspectionTypeAttendance:
			if _, ok := included[record.TypeCode]; !ok || strings.TrimSpace(record.StudentNumber) == "" {
				continue
			}
			attendancePeople[record.StudentNumber] = struct{}{}
			for _, class := range classes {
				byClass[class.key].attendancePeople[record.StudentNumber] = struct{}{}
			}
		case inspection.InspectionTypeUnattended:
			if _, ok := included[record.TypeCode]; !ok {
				continue
			}
			if studentNumber := strings.TrimSpace(record.StudentNumber); studentNumber != "" {
				unattendedPeople[studentNumber] = struct{}{}
				for _, class := range classes {
					byClass[class.key].unattendedPeople[studentNumber] = struct{}{}
				}
			}
			for _, class := range classes {
				byClass[class.key].unattendedRaw++
			}
		case inspection.InspectionTypeMorning:
			if _, ok := included[record.TypeCode]; !ok {
				continue
			}
			for _, class := range classes {
				byClass[class.key].morningRaw++
			}
		case inspection.InspectionTypeEveningSelfStudy:
			if _, ok := included[record.TypeCode]; !ok {
				continue
			}
			for _, class := range classes {
				byClass[class.key].eveningRaw++
			}
		case inspection.InspectionTypeClassroomHygiene:
			if _, ok := included[record.TypeCode]; !ok || record.Details.Result != inspection.HygieneUnqualified {
				continue
			}
			for _, class := range classes {
				byClass[class.key].classroomHygieneRaw++
			}
		case inspection.InspectionTypeDormitoryHygiene:
			if _, ok := included[record.TypeCode]; !ok || record.Details.Result != inspection.HygieneUnqualified {
				continue
			}
			for _, class := range classes {
				byClass[class.key].dormitoryPersonalRaw++
			}
		case inspection.InspectionTypePublicDormitoryHygiene:
			if _, ok := included[record.TypeCode]; !ok || record.Details.Result != inspection.HygieneUnqualified {
				continue
			}
			for _, class := range classes {
				byClass[class.key].dormitoryPublicRaw++
			}
		case inspection.InspectionTypeLateReturn:
			if _, ok := included[record.TypeCode]; !ok {
				continue
			}
			for _, class := range classes {
				byClass[class.key].lateReturnRaw++
			}
		}
	}

	codes := make([]string, 0, len(included))
	for code := range included {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	classKeys := make([]string, 0, len(byClass))
	for key := range byClass {
		classKeys = append(classKeys, key)
	}
	sort.Slice(classKeys, func(i, j int) bool {
		left, right := byClass[classKeys[i]], byClass[classKeys[j]]
		if left.grade != right.grade {
			return left.grade < right.grade
		}
		return left.className < right.className
	})

	result := &ReportIndicators{
		Inputs: inputs, IncludedTypeCodes: codes,
		Classes: make([]ReportClassIndicators, 0, len(classKeys)),
	}
	var totalUnattendedRaw, totalMorningRaw int
	var totalEveningRaw, totalClassroomHygieneRaw, totalDormitoryPersonalRaw int
	var totalDormitoryPublicRaw, totalLateReturnRaw int
	for _, key := range classKeys {
		accumulator := byClass[key]
		values := buildIndicatorValues(inputs, included, accumulator.expectedStudents,
			len(accumulator.attendancePeople),
			len(accumulator.unattendedPeople), accumulator.unattendedRaw,
			accumulator.morningRaw, accumulator.eveningRaw, accumulator.classroomHygieneRaw,
			accumulator.dormitoryPersonalRaw, accumulator.dormitoryPublicRaw, accumulator.lateReturnRaw)
		result.Classes = append(result.Classes, ReportClassIndicators{
			ClassID: accumulator.classID, Grade: accumulator.grade, ClassName: accumulator.className, ReportIndicatorValues: values,
		})
		if inputs.ReportGradeScopeVersion > 0 && values.AttendanceRate != nil && values.AttendanceRate.ViolationStudentCount > accumulator.expectedStudents {
			return nil, fmt.Errorf("%w: attendance count exceeds class roster", ErrReportMetricScopeInvalid)
		}
		totalUnattendedRaw += accumulator.unattendedRaw
		totalMorningRaw += accumulator.morningRaw
		totalEveningRaw += accumulator.eveningRaw
		totalClassroomHygieneRaw += accumulator.classroomHygieneRaw
		totalDormitoryPersonalRaw += accumulator.dormitoryPersonalRaw
		totalDormitoryPublicRaw += accumulator.dormitoryPublicRaw
		totalLateReturnRaw += accumulator.lateReturnRaw
	}
	result.Totals = buildIndicatorValues(inputs, included, totalExpectedStudents,
		len(attendancePeople), len(unattendedPeople), totalUnattendedRaw,
		totalMorningRaw, totalEveningRaw, totalClassroomHygieneRaw,
		totalDormitoryPersonalRaw, totalDormitoryPublicRaw, totalLateReturnRaw)
	if inputs.ReportGradeScopeVersion > 0 && result.Totals.AttendanceRate != nil && result.Totals.AttendanceRate.ViolationStudentCount > totalExpectedStudents {
		return nil, fmt.Errorf("%w: attendance count exceeds semester roster", ErrReportMetricScopeInvalid)
	}
	return result, nil
}

// RecalculateReportIndicators preserves the version's inputs and recomputes only
// its derived metrics after a correction or a new include/exclude version.
func RecalculateReportIndicators(content *ReportVersionContent, records []Record) error {
	if content.Indicators == nil {
		return nil
	}
	indicators, err := CalculateReportIndicators(content.Indicators.Inputs, content.Rows, records)
	if err != nil {
		return err
	}
	content.Indicators = indicators
	ApplyReportSourceStates(content)
	return nil
}

// ApplyReportSourceStates describes source completeness at submission-cell granularity.
// A class with zero records shares its type's confirmed source, but this is not
// evidence that the class was individually inspected.
func ApplyReportSourceStates(content *ReportVersionContent) {
	if content.Indicators == nil || content.StatisticsRuleVersion != "WEEKLY_INDICATORS_V2" {
		return
	}
	states := make(map[string]string, len(content.Indicators.IncludedTypeCodes))
	for _, code := range content.Indicators.IncludedTypeCodes {
		states[code] = "CONFIRMED_ZERO"
	}
	for _, row := range content.Rows {
		code := row.InspectionTypeCode
		if code == "" {
			code = reportTypeCodeForName(row.InspectionType)
		}
		if code == "" {
			continue
		}
		switch {
		case row.DataStatus == "NO_CONFIRMED_DATA":
			states[code] = "SOURCE_INCOMPLETE"
		case row.RecordCount > 0 && states[code] != "SOURCE_INCOMPLETE":
			states[code] = "CONFIRMED_WITH_RECORDS"
		}
	}
	for _, cell := range content.IncompleteCells {
		for _, row := range content.Rows {
			if row.InspectionTypeID == cell.InspectionTypeID {
				states[row.InspectionTypeCode] = "SOURCE_INCOMPLETE"
			}
		}
	}
	content.Indicators.SourceStates = states
	for i := range content.Indicators.Classes {
		class := &content.Indicators.Classes[i]
		class.SourceStates = make(map[string]string, len(states))
		for code, state := range states {
			if state == "SOURCE_INCOMPLETE" {
				class.SourceStates[code] = state
			} else if classHasReportRecords(*class, code) {
				class.SourceStates[code] = "CONFIRMED_WITH_RECORDS"
			} else {
				class.SourceStates[code] = "CONFIRMED_ZERO"
			}
		}
	}
}

func classHasReportRecords(class ReportClassIndicators, code string) bool {
	switch code {
	case inspection.InspectionTypeAttendance:
		return class.AttendanceRate != nil && class.AttendanceRate.ViolationStudentCount > 0
	case inspection.InspectionTypeUnattended:
		return class.UnattendedClassroom != nil && class.UnattendedClassroom.RawRecordCount > 0
	case inspection.InspectionTypeMorning:
		return class.MorningAttendance != nil && class.MorningAttendance.AbsencePersonTimes > 0
	case inspection.InspectionTypeEveningSelfStudy:
		return class.EveningSelfStudy != nil && class.EveningSelfStudy.RawRecordCount > 0
	case inspection.InspectionTypeClassroomHygiene:
		return class.ClassroomHygiene != nil && class.ClassroomHygiene.RawRecordCount > 0
	case inspection.InspectionTypeDormitoryHygiene:
		return class.DormitoryPersonalHygiene != nil && class.DormitoryPersonalHygiene.RawRecordCount > 0
	case inspection.InspectionTypePublicDormitoryHygiene:
		return class.DormitoryPublicHygiene != nil && class.DormitoryPublicHygiene.RawRecordCount > 0
	case inspection.InspectionTypeLateReturn:
		return class.LateReturn != nil && class.LateReturn.RawRecordCount > 0
	default:
		return false
	}
}

type reportClassReference struct {
	key       string
	grade     string
	className string
}

func reportRecordClasses(record Record) []reportClassReference {
	classes := make([]reportClassReference, 0, len(record.ResponsibleClasses))
	seen := make(map[string]struct{}, len(record.ResponsibleClasses))
	for _, reference := range record.ResponsibleClasses {
		name, grade := strings.TrimSpace(reference.Label), strings.TrimSpace(reference.Grade)
		if name == "" {
			continue
		}
		key := reportClassKey(grade, name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		classes = append(classes, reportClassReference{key: key, grade: grade, className: name})
	}
	if len(classes) > 0 {
		return classes
	}
	name, grade := strings.TrimSpace(record.ClassName), strings.TrimSpace(record.Grade)
	if name == "" {
		return nil
	}
	key := reportClassKey(grade, name)
	return []reportClassReference{{key: key, grade: grade, className: name}}
}

func buildIndicatorValues(inputs ReportMetricInputs, included map[string]struct{}, expectedStudents,
	attendancePeople, unattendedPeople, unattendedRaw, morningRaw, eveningRaw,
	classroomHygieneRaw, dormitoryPersonalRaw, dormitoryPublicRaw, lateReturnRaw int) ReportIndicatorValues {
	values := ReportIndicatorValues{ExpectedStudentCount: expectedStudents}
	if _, ok := included[inspection.InspectionTypeAttendance]; ok {
		attended := expectedStudents - attendancePeople
		values.AttendanceRate = &ReportAttendanceRateMetric{
			ViolationStudentCount: attendancePeople, ExpectedStudentCount: expectedStudents,
			AttendedStudentCount: attended, Rate: percentage(attended, expectedStudents),
		}
	}
	if _, ok := included[inspection.InspectionTypeUnattended]; ok {
		values.UnattendedClassroom = &ReportUnattendedMetric{
			RawRecordCount: unattendedRaw, ViolationCount: unattendedRaw * inputs.WeekMultiplier,
			ViolationPersonTimeCount: unattendedRaw * inputs.WeekMultiplier,
			ViolatingStudentCount:    unattendedPeople,
		}
	}
	if _, ok := included[inspection.InspectionTypeMorning]; ok {
		expectedPersonTimes := expectedStudents * len(inputs.MorningPlannedDates)
		attendedPersonTimes := expectedPersonTimes - morningRaw
		values.MorningAttendance = &ReportMorningAttendanceMetric{
			PlannedDayCount: len(inputs.MorningPlannedDates), ExpectedPersonTimes: expectedPersonTimes,
			AbsencePersonTimes: morningRaw, AttendedPersonTimes: attendedPersonTimes,
			PenaltyCount: morningRaw * inputs.WeekMultiplier, Rate: percentage(attendedPersonTimes, expectedPersonTimes),
		}
	}
	if _, ok := included[inspection.InspectionTypeEveningSelfStudy]; ok {
		values.EveningSelfStudy = weightedPersonTimeMetric(eveningRaw, inputs.WeekMultiplier)
	}
	if _, ok := included[inspection.InspectionTypeClassroomHygiene]; ok {
		values.ClassroomHygiene = weightedMetric(classroomHygieneRaw, inputs.WeekMultiplier)
	}
	if _, ok := included[inspection.InspectionTypeDormitoryHygiene]; ok {
		values.DormitoryPersonalHygiene = weightedMetric(dormitoryPersonalRaw, inputs.WeekMultiplier)
	}
	if _, ok := included[inspection.InspectionTypePublicDormitoryHygiene]; ok {
		values.DormitoryPublicHygiene = weightedMetric(dormitoryPublicRaw, inputs.WeekMultiplier)
	}
	if _, personalIncluded := included[inspection.InspectionTypeDormitoryHygiene]; personalIncluded {
		if _, publicIncluded := included[inspection.InspectionTypePublicDormitoryHygiene]; publicIncluded {
			values.DormitoryHygieneTotal = weightedMetric(dormitoryPersonalRaw+dormitoryPublicRaw, inputs.WeekMultiplier)
		}
	}
	if _, ok := included[inspection.InspectionTypeLateReturn]; ok {
		values.LateReturn = weightedPersonTimeMetric(lateReturnRaw, inputs.WeekMultiplier)
	}
	return values
}

func weightedMetric(rawCount, multiplier int) *ReportCountMetric {
	return &ReportCountMetric{RawRecordCount: rawCount, Count: rawCount * multiplier}
}

func weightedPersonTimeMetric(rawCount, multiplier int) *ReportCountMetric {
	metric := weightedMetric(rawCount, multiplier)
	personTimeCount := metric.Count
	metric.PersonTimeCount = &personTimeCount
	return metric
}

func percentage(numerator, denominator int) *float64 {
	if denominator == 0 {
		return nil
	}
	value := math.Round((float64(numerator)/float64(denominator)*100)*100) / 100
	return &value
}

func newIndicatorAccumulator(grade, className string) *indicatorAccumulator {
	return &indicatorAccumulator{
		grade: grade, className: className,
		attendancePeople: make(map[string]struct{}), unattendedPeople: make(map[string]struct{}),
	}
}

func reportClassKey(grade, className string) string {
	return strings.TrimSpace(grade) + "\x1f" + strings.TrimSpace(className)
}

func reportTypeCodeForName(name string) string {
	return map[string]string{
		"到课率":    inspection.InspectionTypeAttendance,
		"无机课堂":   inspection.InspectionTypeUnattended,
		"早锻炼/早签": inspection.InspectionTypeMorning,
		"晚自习":    inspection.InspectionTypeEveningSelfStudy,
		"教室卫生":   inspection.InspectionTypeClassroomHygiene,
		"宿舍卫生":   inspection.InspectionTypeDormitoryHygiene,
		"公共宿舍卫生": inspection.InspectionTypePublicDormitoryHygiene,
		"晚归":     inspection.InspectionTypeLateReturn,
	}[strings.TrimSpace(name)]
}
