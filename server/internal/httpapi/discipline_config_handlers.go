package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/disciplineconfig"
	"github.com/gin-gonic/gin"
)

func (h *handler) requireSystemAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := h.currentAccount(c)
		if !ok {
			c.Abort()
			return
		}
		if (a.SystemRole != auth.SystemRoleAdmin && !a.IsSuperAdmin) || a.MustChangePassword {
			writeError(c, 403, "FORBIDDEN", "无权访问该功能")
			c.Abort()
			return
		}
		c.Set("account", a)
		c.Next()
	}
}

func (h *handler) disciplineActor(c *gin.Context) (auth.Account, bool) {
	a, ok := h.currentAccount(c)
	if !ok {
		return auth.Account{}, false
	}
	if h.disciplineConfig == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "处分配置服务尚未就绪")
		return auth.Account{}, false
	}
	return a, true
}
func (h *handler) currentDisciplineRules(c *gin.Context) {
	if _, ok := h.disciplineActor(c); !ok {
		return
	}
	v, exists, err := h.disciplineConfig.Current(c)
	if err != nil {
		writeDisciplineError(c, err)
		return
	}
	if !exists {
		c.JSON(200, gin.H{"current": nil})
		return
	}
	c.JSON(200, gin.H{"current": v})
}
func (h *handler) listDisciplineRuleVersions(c *gin.Context) {
	if _, ok := h.disciplineActor(c); !ok {
		return
	}
	v, err := h.disciplineConfig.ListVersions(c)
	if err != nil {
		writeDisciplineError(c, err)
		return
	}
	c.JSON(200, gin.H{"items": v})
}
func (h *handler) getDisciplineRuleVersion(c *gin.Context) {
	if _, ok := h.disciplineActor(c); !ok {
		return
	}
	v, err := h.disciplineConfig.GetVersion(c, c.Param("id"))
	if err != nil {
		writeDisciplineError(c, err)
		return
	}
	c.JSON(200, v)
}
func (h *handler) createDisciplineRuleDraft(c *gin.Context) {
	a, ok := h.disciplineActor(c)
	if !ok {
		return
	}
	v, err := h.disciplineConfig.CreateDraft(c, a.ID)
	if err != nil {
		writeDisciplineError(c, err)
		return
	}
	c.JSON(201, v)
}
func (h *handler) saveDisciplineRuleDraft(c *gin.Context) {
	a, ok := h.disciplineActor(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var v disciplineconfig.Version
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil || v.ID != "" && v.ID != c.Param("id") {
		writeError(c, 400, "INVALID_REQUEST", "规则配置格式无效")
		return
	}
	v.ID = c.Param("id")
	saved, err := h.disciplineConfig.SaveDraft(c, v, v.RowVersion, a.ID)
	if err != nil {
		writeDisciplineError(c, err)
		return
	}
	c.JSON(200, saved)
}
func (h *handler) validateDisciplineRuleDraft(c *gin.Context) {
	if _, ok := h.disciplineActor(c); !ok {
		return
	}
	if err := h.disciplineConfig.Validate(c, c.Param("id")); err != nil {
		writeDisciplineError(c, err)
		return
	}
	c.JSON(200, gin.H{"valid": true})
}
func (h *handler) publishDisciplineRuleDraft(c *gin.Context) {
	a, ok := h.disciplineActor(c)
	if !ok {
		return
	}
	var input struct {
		RowVersion int `json:"rowVersion"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if !bind(c, &input) {
		return
	}
	v, err := h.disciplineConfig.Publish(c, c.Param("id"), input.RowVersion, a.ID)
	if err != nil {
		writeDisciplineError(c, err)
		return
	}
	c.JSON(200, v)
}
func writeDisciplineError(c *gin.Context, err error) {
	var validation *disciplineconfig.ValidationError
	if errors.As(err, &validation) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_DISCIPLINE_RULE", "message": "规则不完整，请按字段提示修正后再试", "details": validation.Issues})
		return
	}
	switch {
	case errors.Is(err, disciplineconfig.ErrNotFound):
		writeError(c, 404, "DISCIPLINE_RULE_NOT_FOUND", "处分规则版本不存在")
	case errors.Is(err, disciplineconfig.ErrConflict):
		writeError(c, 409, "DISCIPLINE_RULE_CONFLICT", "处分规则已发生变化，请刷新后重试")
	case errors.Is(err, disciplineconfig.ErrImmutable):
		writeError(c, 409, "DISCIPLINE_RULE_IMMUTABLE", "仅可编辑草稿版本")
	case errors.Is(err, disciplineconfig.ErrInvalidConfig):
		writeError(c, 400, "INVALID_DISCIPLINE_RULE", "规则不完整或存在重叠，请检查累计项和处分区间")
	default:
		writeError(c, 500, "INTERNAL_ERROR", "处分规则操作失败，请稍后重试")
	}
}

func (h *handler) disciplineLevelOptions(c *gin.Context) {
	if h.dictionary == nil {
		writeError(c, 503, "SERVICE_UNAVAILABLE", "字典服务尚未就绪")
		return
	}
	catalog, err := h.dictionary.List(c, "")
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	for _, t := range catalog.Types {
		if t.Code == "DISCIPLINE_LEVEL" && t.Enabled {
			levels, err := h.dictionary.List(c, t.ID)
			writeResult(c, dictionary.Catalog{Types: []dictionary.Type{t}, Items: levels.Items}, err)
			return
		}
	}
	c.JSON(200, dictionary.Catalog{Types: []dictionary.Type{}, Items: []dictionary.Item{}})
}
