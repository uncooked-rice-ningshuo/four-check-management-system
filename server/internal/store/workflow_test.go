package store

import (
	"testing"

	"four-check-management-system/server/internal/inspection"
)

func TestActiveSubmissionTypeIncludesAllEightInspectionKinds(t *testing.T) {
	types := []string{
		inspection.InspectionTypeAttendance,
		inspection.InspectionTypeUnattended,
		inspection.InspectionTypeMorning,
		inspection.InspectionTypeLateReturn,
		inspection.InspectionTypeDormitoryHygiene,
		inspection.InspectionTypeClassroomHygiene,
		inspection.InspectionTypePublicDormitoryHygiene,
		inspection.InspectionTypeEveningSelfStudy,
	}
	for _, typeCode := range types {
		t.Run(typeCode, func(t *testing.T) {
			if !activeSubmissionType(typeCode) {
				t.Fatalf("inspection type %s is not enabled for submissions", typeCode)
			}
		})
	}
}
