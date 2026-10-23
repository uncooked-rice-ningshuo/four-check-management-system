package importjob

import "four-check-management-system/server/internal/inspection"

// InspectionTemplate lists only templates with implemented server adapters.
// Other catalog types remain unavailable until their import path is verified.
func InspectionTemplate(typeCode string) (CourseTemplateDefinition, error) {
	if typeCode == inspection.InspectionTypeClassroomHygiene {
		return CourseTemplateDefinition{TypeCode: typeCode, Code: "CLASSROOM_HYGIENE_IMPORT", Name: "教室卫生导入模板", Sheet: "教室卫生导入", Filename: "classroom-hygiene-v1.xlsx"}, nil
	}
	if typeCode == inspection.InspectionTypeDormitoryHygiene {
		return CourseTemplateDefinition{TypeCode: typeCode, Code: "DORMITORY_HYGIENE_IMPORT", Name: "宿舍卫生导入模板", Sheet: "宿舍卫生导入", Filename: "dormitory-hygiene-v1.xlsx"}, nil
	}
	if typeCode == inspection.InspectionTypeLateReturn {
		return CourseTemplateDefinition{TypeCode: typeCode, Code: "LATE_RETURN_IMPORT", Name: "晚归导入模板", Sheet: "晚归导入", Filename: "late-return-v1.xlsx"}, nil
	}
	if typeCode == inspection.InspectionTypeMorning {
		return CourseTemplateDefinition{TypeCode: typeCode, Code: "MORNING_ATTENDANCE_IMPORT", Name: "早锻炼/早签导入模板", Sheet: "早锻炼早签导入", Filename: "morning-attendance-v1.xlsx"}, nil
	}
	return CourseTemplate(typeCode)
}

// BuildInspectionTemplate generates a supported immutable template version.
func BuildInspectionTemplate(typeCode string) ([]byte, error) {
	if typeCode == inspection.InspectionTypeClassroomHygiene {
		return BuildClassroomHygieneTemplate()
	}
	if typeCode == inspection.InspectionTypeDormitoryHygiene {
		return BuildDormitoryHygieneTemplate()
	}
	if typeCode == inspection.InspectionTypeMorning {
		return BuildMorningTemplate()
	}
	if typeCode == inspection.InspectionTypeLateReturn {
		return BuildLateReturnTemplate()
	}
	return BuildCourseTemplate(typeCode)
}

// NewInspectionXLSXParser applies one safety policy with an exact type contract.
func NewInspectionXLSXParser(typeCode string, maxBytes int64) (*XLSXParser, error) {
	definition, err := InspectionTemplate(typeCode)
	if err != nil {
		return nil, err
	}
	config := NewAttendanceXLSXParser(maxBytes).config
	config.ExpectedTemplateCode, config.ExpectedSheetName = definition.Code, definition.Sheet
	if typeCode == inspection.InspectionTypeClassroomHygiene {
		config.Fields = append([]TemplateField(nil), classroomHygieneImportFields...)
	}
	if typeCode == inspection.InspectionTypeDormitoryHygiene {
		config.Fields = append([]TemplateField(nil), dormitoryHygieneImportFields...)
	}
	if typeCode == inspection.InspectionTypeMorning {
		config.Fields = append([]TemplateField(nil), morningImportFields...)
	}
	if typeCode == inspection.InspectionTypeLateReturn {
		config.Fields = append([]TemplateField(nil), lateReturnImportFields...)
	}
	return NewXLSXParser(config), nil
}
