package httpapi

import (
	"context"
	"net/http"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"github.com/gin-gonic/gin"
)

type importRollbackHTTPService interface {
	Rollback(context.Context, inspection.ManualActor, importjob.RollbackInput, audit.Metadata) (importjob.RollbackResult, error)
}

func (h *handler) rollbackImportBatch(c *gin.Context) {
	account, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if account.MustChangePassword {
		writeError(c, http.StatusForbidden, "FORBIDDEN", "请先修改密码")
		return
	}
	if h.importRollback == nil {
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "回滚服务尚未就绪")
		return
	}
	var input struct {
		Version int    `json:"version"`
		Reason  string `json:"reason"`
	}
	if !bind(c, &input) {
		return
	}
	result, err := h.importRollback.Rollback(c, inspection.ManualActor{ID: account.ID, IsSuperAdmin: account.IsSuperAdmin},
		importjob.RollbackInput{BatchID: c.Param("id"), Version: input.Version, Reason: input.Reason}, requestMetadata(c, input.Reason))
	if err != nil {
		writeImportError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
