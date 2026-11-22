package httpapi

import (
	"errors"
	"strconv"

	"four-check-management-system/server/internal/inbox"
	"github.com/gin-gonic/gin"
)

func (h *handler) notifications(c *gin.Context) {
	a, ok := h.currentAccount(c)
	if !ok {
		return
	}
	if a.MustChangePassword {
		writeError(c, 403, "PASSWORD_CHANGE_REQUIRED", "请先修改初始密码")
		return
	}
	if h.inbox == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "消息服务尚未就绪")
		return
	}
	if c.Request.Method == "POST" {
		var input struct {
			ID  string
			IDs []string
			All bool
		}
		if !bindInspection(c, &input) {
			return
		}
		changed := int64(0)
		if input.All {
			through, err := strconv.ParseInt(input.ID, 10, 64)
			if err != nil || through < 1 {
				writeError(c, 400, "INVALID_REQUEST", "消息编号无效")
				return
			}
			_, count, err := h.inbox.ReadDetailed(c, a.ID, through, true)
			if err != nil {
				writeError(c, 500, "INBOX_FAILED", "更新已读状态失败")
				return
			}
			changed = count
		} else if len(input.IDs) > 0 {
			ids := make([]int64, 0, len(input.IDs))
			for _, raw := range input.IDs {
				id, err := strconv.ParseInt(raw, 10, 64)
				if err != nil || id < 1 {
					writeError(c, 400, "INVALID_REQUEST", "消息编号无效")
					return
				}
				ids = append(ids, id)
			}
			found, count, err := h.inbox.ReadBatch(c, a.ID, ids)
			if err != nil {
				if errors.Is(err, inbox.ErrInvalid) {
					writeError(c, 400, "INVALID_REQUEST", "消息编号无效")
				} else {
					writeError(c, 500, "INBOX_FAILED", "更新已读状态失败")
				}
				return
			}
			if !found {
				writeError(c, 404, "NOT_FOUND", "消息不存在")
				return
			}
			changed = count
		} else {
			id, err := strconv.ParseInt(input.ID, 10, 64)
			if err != nil || id < 1 {
				writeError(c, 400, "INVALID_REQUEST", "消息编号无效")
				return
			}
			found, count, err := h.inbox.ReadDetailed(c, a.ID, id, false)
			if err != nil {
				writeError(c, 500, "INBOX_FAILED", "更新已读状态失败")
				return
			}
			if !found {
				writeError(c, 404, "NOT_FOUND", "消息不存在")
				return
			}
			changed = count
		}
		c.JSON(200, gin.H{"ok": true, "changed": changed})
		return
	}
	page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, sizeErr := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	unread, unreadErr := strconv.ParseBool(c.DefaultQuery("unread", "false"))
	if pageErr != nil || sizeErr != nil || unreadErr != nil {
		writeError(c, 400, "INVALID_REQUEST", "查询条件无效")
		return
	}
	result, err := h.inbox.PageFiltered(c, a.ID, inbox.PageFilter{
		Page: page, Size: size, Unread: unread, Category: c.Query("category"),
		From: c.Query("from"), To: c.Query("to"), Sort: c.Query("sort"),
	})
	if errors.Is(err, inbox.ErrInvalid) {
		writeError(c, 400, "INVALID_REQUEST", "查询条件无效")
		return
	}
	if err != nil {
		writeError(c, 500, "INBOX_FAILED", "读取消息失败")
		return
	}
	c.Data(200, "application/json", result)
}
