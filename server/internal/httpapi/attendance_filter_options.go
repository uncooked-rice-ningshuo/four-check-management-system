package httpapi

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"github.com/gin-gonic/gin"
	"net/http"
)

// Keep the legacy response shape, but project every option through the same
// SQL-authorized context service used by the other inspection pages.
func (h *handler) attendanceFilterOptions(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if account.MustChangePassword {
		writeError(c, 403, "FORBIDDEN", "修改初始密码后才能访问检查业务")
		return
	}
	if h.organization == nil || h.academic == nil || h.inspectionContextOptions == nil || (!account.IsSuperAdmin && h.permission == nil) {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "筛选选项服务尚未就绪")
		return
	}
	catalogs, err := h.organization.ListCatalogs(c, true)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	calendar, err := h.academic.List(c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	kind, found := attendanceInspectionType(catalogs.InspectionTypes)
	if !found {
		writeError(c, 503, "ATTENDANCE_TYPE_UNAVAILABLE", "到课率检查类型尚未启用")
		return
	}
	if !account.IsSuperAdmin {
		effective, err := h.permission.Effective(c, account.ID)
		if err != nil {
			writeError(c, 500, "PERMISSION_LOOKUP_FAILED", "无法读取当前权限")
			return
		}
		allowed := false
		for _, p := range effective.Permissions {
			if p.Action == permission.ActionInspectionView && p.Effect == permission.EffectAllow && (p.Scope.InspectionTypeID == "" || p.Scope.InspectionTypeID == kind.ID) {
				allowed = true
				break
			}
		}
		if !allowed {
			writeInspectionForbidden(c, false)
			return
		}
	}
	actor := inspection.ManualActor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin}
	result := attendanceFilterOptionsResponse{InspectionType: kind, ResponsibleDepartments: []organization.Department{}, Departments: []organization.Department{}, Semesters: []academic.Semester{}, Weeks: []academic.Week{}, Classes: []attendanceClassOption{}, Grades: []string{}}
	options := map[string]map[string]inspection.EntryOption{}
	for _, name := range []string{"semesters", "departments", "classes", "grades"} {
		values, e := h.authorizedAttendanceOptions(c, actor, kind.ID, name, "")
		if e != nil {
			writeInspectionError(c, e)
			return
		}
		options[name] = map[string]inspection.EntryOption{}
		for _, v := range values {
			options[name][v.ID] = v
			if name == "classes" {
				result.Classes = append(result.Classes, attendanceClassOption{ID: v.ID, Name: v.Label, Grade: v.Grade})
			}
			if name == "grades" {
				result.Grades = append(result.Grades, v.Label)
			}
		}
	}
	for _, d := range catalogs.Departments {
		if _, ok := options["departments"][d.ID]; ok && d.Enabled {
			result.Departments = append(result.Departments, d)
		}
	}
	if _, allowed := options["departments"][inspection.CollegeDepartmentOptionID]; allowed {
		result.Departments = append(result.Departments, organization.Department{ID: inspection.CollegeDepartmentOptionID, Code: "COLLEGE", Name: "校级", Enabled: true})
	}
	weekIDs := map[string]bool{}
	for _, s := range calendar.Semesters {
		if _, ok := options["semesters"][s.ID]; !ok || !s.Enabled {
			continue
		}
		result.Semesters = append(result.Semesters, s)
		weeks, e := h.authorizedAttendanceOptions(c, actor, kind.ID, "weeks", s.ID)
		if e != nil {
			writeInspectionError(c, e)
			return
		}
		for _, w := range weeks {
			weekIDs[w.ID] = true
		}
	}
	for _, w := range calendar.Weeks {
		if w.Enabled && weekIDs[w.ID] {
			result.Weeks = append(result.Weeks, w)
		}
	}
	scopes, err := h.organization.ListScopes(c)
	if err != nil {
		writeDomainError(c, err)
		return
	}
	result.ResponsibleDepartments = attendanceResponsibleDepartments(result.Departments, scopes, kind.ID)
	c.JSON(http.StatusOK, result)
}

func (h *handler) authorizedAttendanceOptions(ctx context.Context, actor inspection.ManualActor, typeID, kind, semesterID string) ([]inspection.EntryOption, error) {
	result := []inspection.EntryOption{}
	seen := map[string]bool{}
	for page := 1; page <= 1000000; page++ {
		values, err := h.inspectionContextOptions.ContextOptions(ctx, actor, inspection.ContextOptionQuery{TypeCode: inspection.InspectionTypeAttendance, Purpose: "view", Kind: kind, SemesterID: semesterID, Page: page, PageSize: 200})
		if err != nil {
			return nil, err
		}
		if values.InspectionTypeID != typeID || values.Page != page || values.Total < 0 {
			return nil, errors.New("inconsistent authorized options")
		}
		for _, v := range values.Items {
			if v.ID == "" || seen[v.ID] || (kind == "classes" && v.Grade == "") {
				return nil, errors.New("invalid authorized option")
			}
			seen[v.ID] = true
			result = append(result, v)
		}
		if len(result) == values.Total {
			return result, nil
		}
		if len(values.Items) == 0 || len(result) > values.Total {
			return nil, errors.New("incomplete authorized options")
		}
	}
	return nil, errors.New("authorized options page limit exceeded")
}
