package academic

import (
	"github.com/google/uuid"
	"testing"
)

func TestYearRosterAcceptsDifferentCohortMajorsAndRejectsAmbiguity(t *testing.T) {
	m1, m2 := uuid.NewString(), uuid.NewString()
	class := uuid.NewString()
	r := YearRoster{MajorIDs: []string{m2, m1}, Grades: []RosterGrade{
		{GradeID: uuid.NewString(), Majors: []RosterMajor{{MajorID: m1, ClassIDs: []string{class}}}},
		{GradeID: uuid.NewString(), Majors: []RosterMajor{{MajorID: m2, ClassIDs: []string{}}}},
	}}
	if !validRoster(&r) {
		t.Fatal("different cohort programs rejected")
	}
	r.Grades[1].Majors[0].ClassIDs = []string{class}
	if validRoster(&r) {
		t.Fatal("same class assigned twice")
	}
	r.Grades[1].Majors[0] = RosterMajor{MajorID: uuid.NewString()}
	if validRoster(&r) {
		t.Fatal("major outside ordered list accepted")
	}
}
