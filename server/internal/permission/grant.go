package permission

import (
	"errors"
	"github.com/google/uuid"
	"slices"
	"time"
)

var ErrOperationRangeConflict = errors.New("class does not match operation range")

var ErrGradeMappingMissing = errors.New("academic year grade-stage mapping is missing")

// OperationRange keeps conjunctive dimensions together; separate ranges are alternatives.
type OperationRange struct {
	GradeID    string `json:"gradeId,omitempty"`
	MajorID    string `json:"majorId,omitempty"`
	ClassID    string `json:"classId,omitempty"`
	GradeStage int    `json:"gradeStage,omitempty"`
}

// rangesCover identifies a complete denial without guessing class/grade relationships.
func rangesCover(denied, allowed []OperationRange) bool {
	if len(denied) == 0 {
		return true
	}
	if len(allowed) == 0 {
		allowed = []OperationRange{{}}
	}
	for _, a := range allowed {
		covered := false
		for _, d := range denied {
			if (d.GradeID == "" || d.GradeID == a.GradeID) && (d.MajorID == "" || d.MajorID == a.MajorID) && (d.ClassID == "" || d.ClassID == a.ClassID) && (d.GradeStage == 0 || d.GradeStage == a.GradeStage) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// OperationTarget uses the record's academic-year mapping, never today's semester.
type OperationTarget struct {
	GradeID, MajorID, ClassID string
	StageGrades               map[int]string
}

// MatchesOperationRanges checks whole clauses without mixing dimensions between grants.
func MatchesOperationRanges(ranges []OperationRange, target OperationTarget) bool {
	if len(ranges) == 0 {
		return true
	}
	for _, scope := range ranges {
		if scope.GradeStage < 0 || scope.GradeStage > 4 {
			continue
		}
		if scope.GradeStage != 0 {
			grade, ok := target.StageGrades[scope.GradeStage]
			if !ok || grade == "" || grade != target.GradeID {
				continue
			}
		}
		if (scope.GradeID == "" || scope.GradeID == target.GradeID) &&
			(scope.MajorID == "" || scope.MajorID == target.MajorID) &&
			(scope.ClassID == "" || scope.ClassID == target.ClassID) {
			return true
		}
	}
	return false
}

// GrantAction is an enabled template action with its own operation scope.
type GrantAction struct {
	Action           string           `json:"action"`
	InspectionTypeID string           `json:"inspectionTypeId,omitempty"`
	Ranges           []OperationRange `json:"ranges,omitempty"`
}

// GrantTemplate is copied at assignment so later template edits cannot alter users.
type GrantTemplate struct {
	Version       int           `json:"version"`
	ValidityYears int           `json:"validityYears"`
	Actions       []GrantAction `json:"actions"`
}

// AssignedGrant records an immutable baseline; personal overrides are applied separately.
type AssignedGrant struct {
	GrantAction
	DepartmentID    string     `json:"departmentId"`
	TemplateVersion int        `json:"templateVersion"`
	StartsAt        time.Time  `json:"startsAt"`
	EndsAt          *time.Time `json:"endsAt,omitempty"`
}

// ActiveFor uses an exclusive upper bound. Date-only inputs must first include their final day.
func (g AssignedGrant) ActiveFor(departmentID string, now time.Time) bool {
	return g.DepartmentID == departmentID && !g.StartsAt.IsZero() && !now.Before(g.StartsAt) && (g.EndsAt == nil || now.Before(*g.EndsAt))
}

// Assign creates independent baseline values; zero years means no expiration.
func (t GrantTemplate) Assign(departmentID string, startsAt time.Time) ([]AssignedGrant, error) {
	if departmentID == "" || startsAt.IsZero() || t.Version < 1 || t.ValidityYears < 0 || t.ValidityYears > 9999-startsAt.Year() {
		return nil, errors.New("invalid grant assignment")
	}
	grants := make([]AssignedGrant, 0, len(t.Actions))
	for _, action := range t.Actions {
		if !slices.Contains(allActions, action.Action) {
			return nil, errors.New("invalid grant action")
		}
		for _, scope := range action.Ranges {
			if scope.GradeStage != 0 {
				return nil, errors.New("invalid grade stage")
			}
		}
		action.Ranges = append([]OperationRange(nil), action.Ranges...)
		grant := AssignedGrant{GrantAction: action, DepartmentID: departmentID, TemplateVersion: t.Version, StartsAt: startsAt}
		if t.ValidityYears > 0 {
			end := startsAt.AddDate(t.ValidityYears, 0, 0)
			grant.EndsAt = &end
		}
		grants = append(grants, grant)
	}
	return grants, nil
}

// SupportsOperationRange is shared by grant validation and the action catalog.
func SupportsOperationRange(action string) bool {
	return action == ActionInspectionEnter || action == ActionInspectionManage || action == ActionInspectionReview
}

// ForDepartment keeps each route's action/range tuple intact, while retaining explicit global denials.
func (r EffectiveResult) ForDepartment(departmentID string) EffectiveResult {
	filtered := r
	filtered.Permissions = nil
	for _, entry := range r.Permissions {
		if entry.SourceDepartmentID == departmentID || (entry.SourceDepartmentID == "" && (entry.Effect == EffectDeny || r.SystemRole != SystemRoleMember)) {
			filtered.Permissions = append(filtered.Permissions, entry)
		}
	}
	return filtered
}

// ValidOperationRanges rejects malformed dimensions before they reach SQL casts.
func ValidOperationRanges(ranges []OperationRange) bool {
	for _, r := range ranges {
		if r.GradeStage != 0 {
			return false
		}
		if r.GradeID == "" && r.MajorID == "" && r.ClassID == "" {
			return false
		}
		for _, id := range []string{r.GradeID, r.MajorID, r.ClassID} {
			if id != "" {
				if _, err := uuid.Parse(id); err != nil {
					return false
				}
			}
		}
	}
	return true
}

// MissingStageMapping identifies unresolved configured stages after an operation was denied.
func MissingStageMapping(entries []EffectivePermission, target Scope) bool {
	for _, entry := range entries {
		if entry.Effect != EffectAllow || !SupportsOperationRange(entry.Action) || !scopeMatches(entry.Scope, target) {
			continue
		}
		for _, r := range entry.Ranges {
			if r.GradeStage > 0 && target.StageGrades[r.GradeStage] == "" {
				return true
			}
		}
	}
	return false
}

// ModuleValidity gives all default actions in one business module a shared term.
// It is copied in the assignment snapshot; later template changes never renew it.
type ModuleValidity struct {
	ModuleKey     string     `json:"moduleKey"`
	ValidityYears int        `json:"validityYears"`
	EndsAt        *time.Time `json:"endsAt,omitempty"`
}
