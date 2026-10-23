package importjob

// NewCourseImportService selects the parser from the batch's server-owned type,
// preserving separate templates while reusing the approved shared course model.
func NewCourseImportService(repository Repository, files VerifiedFileReader, validator AttendanceRowValidator, maxBytes int64) *Service {
	service := NewService(repository, files, NewAttendanceXLSXParser(maxBytes), validator)
	service.parsers = map[string]WorkbookParser{
		"ATTENDANCE_RATE":      NewAttendanceXLSXParser(maxBytes),
		"UNATTENDED_CLASSROOM": NewUnattendedXLSXParser(maxBytes),
	}
	return service
}

// CourseTemplateDefinition identifies one of the two supported course imports.
type CourseTemplateDefinition struct{ TypeCode, Code, Name, Sheet, Filename string }

// CourseTemplate validates a server type before any template publication or upload.
func CourseTemplate(typeCode string) (CourseTemplateDefinition, error) {
	switch typeCode {
	case "ATTENDANCE_RATE":
		return CourseTemplateDefinition{typeCode, "ATTENDANCE_IMPORT", "到课率导入模板", "到课率导入", "attendance-v1.xlsx"}, nil
	case "UNATTENDED_CLASSROOM":
		return CourseTemplateDefinition{typeCode, "UNATTENDED_CLASSROOM_IMPORT", "无机课堂导入模板", "无机课堂导入", "unattended-classroom-v1.xlsx"}, nil
	default:
		return CourseTemplateDefinition{}, ErrInvalidBatch
	}
}

// BuildCourseTemplate generates only the approved shared course-field format.
func BuildCourseTemplate(typeCode string) ([]byte, error) {
	definition, err := CourseTemplate(typeCode)
	if err != nil {
		return nil, err
	}
	return buildCourseTemplate(definition.Sheet, definition.Code)
}
