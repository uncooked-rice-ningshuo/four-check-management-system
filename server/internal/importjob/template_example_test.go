package importjob

import (
	"bytes"
	"testing"

	"four-check-management-system/server/internal/inspection"
	"github.com/xuri/excelize/v2"
)

func TestExampleTemplateCopyKeepsPublishedVersionAndSkipsExample(t *testing.T) {
	for _, typeCode := range []string{
		inspection.InspectionTypeAttendance, inspection.InspectionTypeUnattended,
		inspection.InspectionTypeMorning, inspection.InspectionTypeDormitoryHygiene,
		inspection.InspectionTypeLateReturn, inspection.InspectionTypeClassroomHygiene,
	} {
		t.Run(typeCode, func(t *testing.T) {
			original, err := BuildInspectionTemplate(typeCode)
			if err != nil {
				t.Fatal(err)
			}
			copyBytes, err := ExampleTemplateCopy(original)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(copyBytes, original) {
				t.Fatal("example copy did not add visible content")
			}
			definition, err := InspectionTemplate(typeCode)
			if err != nil {
				t.Fatal(err)
			}
			parser, err := NewInspectionXLSXParser(typeCode, 16<<20)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parser.Parse(definition.Filename, copyBytes)
			if err != nil || parsed.TemplateVersion != 1 || len(parsed.Rows) != 0 {
				t.Fatalf("example was imported: %+v %v", parsed, err)
			}
			book, err := excelize.OpenReader(bytes.NewReader(copyBytes))
			if err != nil {
				t.Fatal(err)
			}
			defer book.Close()
			rows, err := book.GetRows(definition.Sheet)
			if err != nil || len(rows) < 4 || len(rows[3]) < 2 || rows[3][0] == "" || rows[3][1] == "" {
				t.Fatalf("example row is incomplete: %v %v", rows, err)
			}
		})
	}
}
