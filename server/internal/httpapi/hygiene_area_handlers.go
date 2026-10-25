package httpapi

import (
	"net/http"
	"strconv"

	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/permission"
	"github.com/gin-gonic/gin"
)

type hygieneAreaTarget struct {
	Kind               string `json:"kind"`
	DepartmentID       string `json:"departmentId"`
	DepartmentName     string `json:"departmentName"`
	InspectionTypeID   string `json:"inspectionTypeId"`
	InspectionTypeName string `json:"inspectionTypeName"`
}

// The target list is derived from live mappings and type-scoped permissions so
// area maintenance does not open the general master-data catalog.
func (h *handler) hygieneAreaOptions(c *gin.Context) {
	a, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if a.MustChangePassword {
		writeError(c, http.StatusForbidden, "FORBIDDEN", "无权访问该功能")
		return
	}
	if h.organization == nil || h.permission == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "区域配置服务尚未就绪")
		return
	}
	catalogs, err := h.organization.ListCatalogs(c, true)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	scopes, err := h.organization.ListScopes(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	effective, err := h.permission.Effective(c, a.ID)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	result := make([]hygieneAreaTarget, 0)
	for _, scope := range scopes {
		if !scope.Enabled || !a.IsSuperAdmin && !effective.Allowed(permission.ActionMasterDataManage, permission.Scope{DepartmentID: scope.DepartmentID, InspectionTypeID: scope.InspectionTypeID}) {
			continue
		}
		for _, inspectionType := range catalogs.InspectionTypes {
			if inspectionType.ID != scope.InspectionTypeID {
				continue
			}
			kind := ""
			switch inspectionType.Code {
			case "CLASSROOM_HYGIENE":
				kind = masterdata.KindClassroomHygieneArea
			case "PUBLIC_DORMITORY_HYGIENE":
				kind = masterdata.KindPublicDormitoryHygieneArea
			}
			if kind == "" {
				continue
			}
			for _, department := range catalogs.Departments {
				if department.ID == scope.DepartmentID {
					result = append(result, hygieneAreaTarget{kind, department.ID, department.Name, inspectionType.ID, inspectionType.Name})
					break
				}
			}
			break
		}
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) hygieneAreaReferences(c *gin.Context) {
	if h.masterData == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "区域配置服务尚未就绪")
		return
	}
	parentKind := ""
	switch c.Param("kind") {
	case masterdata.KindClassroomHygieneArea:
		parentKind = masterdata.KindClassroom
	case masterdata.KindPublicDormitoryHygieneArea:
		parentKind = masterdata.KindDormitoryBuilding
	default:
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "区域类型无效")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "页码无效")
		return
	}
	result, err := h.masterData.List(c, masterdata.Query{Kind: parentKind, Enabled: boolPointer(true), Page: page, PageSize: 100})
	writeResult(c, result, err)
}

func boolPointer(value bool) *bool { return &value }
