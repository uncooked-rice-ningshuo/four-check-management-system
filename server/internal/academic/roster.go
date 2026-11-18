package academic

import "github.com/google/uuid"

// YearRoster uses array positions as the authoritative display order.
type YearRoster struct {
	MajorIDs []string      `json:"majorIds"`
	Grades   []RosterGrade `json:"grades"`
}
type RosterGrade struct {
	GradeID string        `json:"gradeId"`
	Majors  []RosterMajor `json:"majors"`
}
type RosterMajor struct {
	MajorID  string   `json:"majorId"`
	ClassIDs []string `json:"classIds"`
}

func addRosterID(seen map[string]bool, id string) bool {
	if uuid.Validate(id) != nil || seen[id] {
		return false
	}
	seen[id] = true
	return true
}
func validRoster(r *YearRoster) bool {
	if r == nil {
		return true
	}
	if len(r.Grades) == 0 || len(r.MajorIDs) == 0 {
		return false
	}
	majors, grades, classes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, id := range r.MajorIDs {
		if !addRosterID(majors, id) {
			return false
		}
	}
	for _, g := range r.Grades {
		if !addRosterID(grades, g.GradeID) || len(g.Majors) == 0 {
			return false
		}
		selected := map[string]bool{}
		for _, m := range g.Majors {
			if !majors[m.MajorID] || !addRosterID(selected, m.MajorID) {
				return false
			}
			for _, id := range m.ClassIDs {
				if !addRosterID(classes, id) {
					return false
				}
			}
		}
	}
	return true
}
