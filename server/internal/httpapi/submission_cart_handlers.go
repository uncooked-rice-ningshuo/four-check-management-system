package httpapi

import (
	"net/http"
	"strings"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/gin-gonic/gin"
)

func (h *handler) listSubmissionImportBatches(c *gin.Context) {
	if h.importJobs == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "导入服务尚未就绪")
		return
	}
	account, scope, ok := h.authorizeInspectionRequest(c, permission.ActionInspectionEnter)
	if !ok {
		return
	}
	if _, _, ok = h.authorizeInspectionRequest(c, permission.ActionInspectionView); !ok {
		return
	}
	if strings.TrimSpace(c.Query("departmentId")) == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请明确选择部门或校级范围")
		return
	}
	if account.SystemRole == auth.SystemRoleMember {
		scope.DepartmentID = account.PrimaryDepartmentID
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	result, err := h.importJobs.ListSubmissionBatches(c, account.ID, importjob.SubmissionBatchQuery{
		SemesterID: scope.SemesterID, WeekID: scope.WeekID, InspectionTypeID: scope.InspectionTypeID,
		DepartmentID: scope.DepartmentID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

type submissionBatchRecord struct {
	Record            inspection.InspectionRecord `json:"record"`
	Selectable        bool                        `json:"selectable"`
	UnavailableReason string                      `json:"unavailableReason,omitempty"`
}

type submissionBatchRecordPage struct {
	Items    []submissionBatchRecord `json:"items"`
	Total    int                     `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"pageSize"`
}

func (h *handler) listSubmissionBatchRecords(c *gin.Context) {
	if h.importJobs == nil || h.inspections == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "导入记录服务尚未就绪")
		return
	}
	account, batch, ok := h.authorizedBatch(c, permission.ActionInspectionView)
	if !ok {
		return
	}
	if batch.CreatedBy != account.ID {
		writeImportError(c, importjob.ErrBatchNotFound)
		return
	}
	if _, ok = h.authorizeInspectionScope(c, permission.ActionInspectionEnter, permission.Scope{
		SemesterID: batch.SemesterID, WeekID: batch.WeekID, InspectionTypeID: batch.InspectionTypeID, DepartmentID: batch.DepartmentID,
	}, true); !ok {
		return
	}
	page, pageSize, ok := pagination(c)
	if !ok {
		return
	}
	actor := inspection.ManualActor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin, IsMember: account.SystemRole == auth.SystemRoleMember, PrimaryDepartmentID: account.PrimaryDepartmentID}
	result, err := h.inspections.List(c, actor, inspection.InspectionFilter{
		SemesterID: batch.SemesterID, WeekID: batch.WeekID, InspectionTypeID: batch.InspectionTypeID,
		DepartmentID: batch.DepartmentID, ImportBatchID: batch.ID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeInspectionError(c, err)
		return
	}
	response := submissionBatchRecordPage{Items: make([]submissionBatchRecord, 0, len(result.Items)), Total: result.Total, Page: result.Page, PageSize: result.PageSize}
	for _, record := range result.Items {
		item := submissionBatchRecord{Record: record, Selectable: record.Status == inspection.RecordStatusDraft && !record.HasSubmission && !record.HasReportLock && record.CanEdit}
		if !item.Selectable {
			switch {
			case record.HasSubmission:
				item.UnavailableReason = "ALREADY_SUBMITTED"
			case record.HasReportLock:
				item.UnavailableReason = "REPORT_LOCKED"
			case record.Status != inspection.RecordStatusDraft:
				item.UnavailableReason = "NOT_DRAFT"
			default:
				item.UnavailableReason = "OUT_OF_ACTION_SCOPE"
			}
		}
		response.Items = append(response.Items, item)
	}
	c.JSON(http.StatusOK, response)
}
