package httpapi

import (
	"errors"
	"log"
	"net/http"

	"four-check-management-system/server/internal/workflow"
	"github.com/gin-gonic/gin"
)

func (h *handler) submitDepartmentBatch(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if account.MustChangePassword {
		writeError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return
	}
	if h.workflow == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "部门提交服务尚未就绪")
		return
	}
	var input workflow.SubmitInput
	if !bindInspection(c, &input) {
		return
	}
	result, err := h.workflow.Submit(c, workflow.Actor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin}, input, requestMetadata(c, input.Explanation))
	switch {
	case err == nil:
		c.JSON(http.StatusCreated, result)
	case errors.Is(err, workflow.ErrInvalidSubmit):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "提交范围、记录或幂等键无效")
	case errors.Is(err, workflow.ErrSubmissionScope):
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权提交该检查范围")
	case errors.Is(err, workflow.ErrRecordUnavailable):
		writeError(c, http.StatusNotFound, "INSPECTION_NOT_FOUND", "记录不存在或不可提交")
	case errors.Is(err, workflow.ErrVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "记录已变化，请刷新后重试")
	case errors.Is(err, workflow.ErrIdempotencyReuse):
		writeError(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "该请求标识已用于其他提交")
	default:
		log.Printf("department submit failed: errorType=%T", err)
		writeError(c, http.StatusInternalServerError, "SUBMISSION_FAILED", "部门提交失败，请稍后重试")
	}
}

func (h *handler) listDepartmentBatches(c *gin.Context) {
	actor, ok := h.departmentActor(c)
	if !ok {
		return
	}
	var query workflow.ListQuery
	if c.ShouldBindQuery(&query) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "分页或筛选格式无效")
		return
	}
	page, err := h.workflow.List(c, actor, query)
	if err != nil {
		writeWorkflowReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *handler) getDepartmentBatch(c *gin.Context) {
	actor, ok := h.departmentActor(c)
	if !ok {
		return
	}
	result, err := h.workflow.Detail(c, actor, c.Param("id"))
	if err != nil {
		writeWorkflowReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *handler) departmentActor(c *gin.Context) (workflow.Actor, bool) {
	account, ok := h.currentAccount(c)
	if !ok {
		return workflow.Actor{}, false
	}
	if account.MustChangePassword {
		writeError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return workflow.Actor{}, false
	}
	if h.workflow == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "部门提交服务尚未就绪")
		return workflow.Actor{}, false
	}
	return workflow.Actor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin}, true
}

func writeWorkflowReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workflow.ErrInvalidSubmit):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "提交筛选或标识无效")
	case errors.Is(err, workflow.ErrSubmissionMissing):
		writeError(c, http.StatusNotFound, "SUBMISSION_NOT_FOUND", "提交批次不存在")
	case errors.Is(err, workflow.ErrSubmissionScope):
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权查看提交批次")
	default:
		log.Printf("department read failed: errorType=%T", err)
		writeError(c, http.StatusInternalServerError, "SUBMISSION_FAILED", "部门提交信息加载失败")
	}
}

func (h *handler) returnDepartmentBatch(c *gin.Context) {
	h.decideDepartmentBatch(c, workflow.Return)
}

func (h *handler) confirmDepartmentBatch(c *gin.Context) {
	h.decideDepartmentBatch(c, workflow.Confirm)
}

func (h *handler) decideDepartmentBatch(c *gin.Context, action workflow.Action) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if account.MustChangePassword {
		writeError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return
	}
	if h.workflow == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "部门审核服务尚未就绪")
		return
	}
	var input workflow.DecisionInput
	if !bindInspection(c, &input) {
		return
	}
	result, err := h.workflow.Decide(c, workflow.Actor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin}, c.Param("id"), action, input, requestMetadata(c, input.Reason))
	switch {
	case err == nil:
		c.JSON(http.StatusOK, result)
	case errors.Is(err, workflow.ErrInvalidDecision):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "审核版本或原因无效")
	case errors.Is(err, workflow.ErrSubmissionMissing):
		writeError(c, http.StatusNotFound, "SUBMISSION_NOT_FOUND", "提交批次不存在")
	case errors.Is(err, workflow.ErrSubmissionScope):
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权审核该提交批次")
	case errors.Is(err, workflow.ErrVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "提交批次已变化，请刷新后重试")
	case errors.Is(err, workflow.ErrInvalidTransition):
		writeError(c, http.StatusConflict, "INVALID_SUBMISSION_STATE", "当前状态不能执行此审核操作")
	default:
		log.Printf("department decision failed: action=%s errorType=%T", action, err)
		writeError(c, http.StatusInternalServerError, "SUBMISSION_FAILED", "部门审核失败，请稍后重试")
	}
}

func (h *handler) resubmitDepartmentBatch(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if account.MustChangePassword {
		writeError(c, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return
	}
	if h.workflow == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "部门提交服务尚未就绪")
		return
	}
	var input workflow.ResubmitInput
	if !bindInspection(c, &input) {
		return
	}
	result, err := h.workflow.Resubmit(c, workflow.Actor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin}, c.Param("id"), input, requestMetadata(c, input.Explanation))
	switch {
	case err == nil:
		c.JSON(http.StatusOK, result)
	case errors.Is(err, workflow.ErrInvalidSubmit):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "重提记录或版本无效")
	case errors.Is(err, workflow.ErrSubmissionMissing):
		writeError(c, http.StatusNotFound, "SUBMISSION_NOT_FOUND", "提交批次不存在")
	case errors.Is(err, workflow.ErrSubmissionScope):
		writeError(c, http.StatusForbidden, "FORBIDDEN_SCOPE", "无权重提该检查批次")
	case errors.Is(err, workflow.ErrVersionConflict):
		writeError(c, http.StatusConflict, "VERSION_CONFLICT", "记录或批次已变化，请刷新后重试")
	case errors.Is(err, workflow.ErrInvalidTransition):
		writeError(c, http.StatusConflict, "INVALID_SUBMISSION_STATE", "当前状态不能重新提交")
	default:
		log.Printf("department resubmit failed: errorType=%T", err)
		writeError(c, http.StatusInternalServerError, "SUBMISSION_FAILED", "部门重提失败，请稍后重试")
	}
}
