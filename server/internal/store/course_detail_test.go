package store

import (
	"testing"

	"four-check-management-system/server/internal/inspection"
)

// 无机课堂 stores one violation per record: the week multiplier must reach both
// penalty_multiplier and penalty_count, while 到课率 keeps penalty_count empty and
// continues to rely on the generated penalty_hours column.
func TestCourseDetailSQLSnapshotsUnattendedPenaltyCount(t *testing.T) {
	multiplier := 2
	_, unattended := courseDetailSQL("record-1", inspection.CourseAbsenceDetails{CourseID: "course-1", Period: "1-2", AbsenceHours: 1}, &multiplier, true)
	if unattended[4] != "1-2" {
		t.Fatalf("无机课堂 period = %v, want its normalized section", unattended[4])
	}
	if unattended[5] != &multiplier {
		t.Fatalf("无机课堂 penalty_multiplier = %v, want the week multiplier", unattended[5])
	}
	if unattended[6] != multiplier {
		t.Fatalf("无机课堂 penalty_count = %v, want %d", unattended[6], multiplier)
	}

	_, attendance := courseDetailSQL("record-2", inspection.CourseAbsenceDetails{CourseID: "course-1", Period: "1-2", AbsenceHours: 2}, &multiplier, false)
	if attendance[6] != nil {
		t.Fatalf("到课率 penalty_count must stay empty, got %v", attendance[6])
	}
}
