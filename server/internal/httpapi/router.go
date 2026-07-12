package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"four-check-management-system/server/internal/auth"
	"github.com/gin-gonic/gin"
)

type handler struct {
	auth         *auth.Service
	secureCookie bool
}

// NewRouter 构建认证模块的 HTTP 适配层。
// 生产 HTTPS 环境必须启用 secureCookie，本地 HTTP 开发环境才允许显式关闭。
func NewRouter(svc *auth.Service, secureCookie bool) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	h := &handler{auth: svc, secureCookie: secureCookie}
	r.GET("/health/live", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	v1 := r.Group("/api/v1/auth")
	v1.POST("/register", h.register)
	v1.POST("/login", h.login)
	v1.POST("/refresh", h.refresh)
	v1.POST("/logout", h.logout)
	v1.GET("/me", h.me)
	return r
}

func (h *handler) register(c *gin.Context) {
	var in struct {
		StudentNo string `json:"studentNo"`
		Name      string `json:"name"`
		Password  string `json:"password"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请检查注册信息")
		return
	}
	a, err := h.auth.Register(c, auth.RegisterInput{StudentNo: in.StudentNo, Name: in.Name, Password: in.Password})
	if errors.Is(err, auth.ErrStudentNoExists) {
		writeError(c, http.StatusConflict, "STUDENT_NO_EXISTS", "该学号已注册")
		return
	}
	if err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REGISTRATION", "学号、姓名不能为空，密码至少 8 位")
		return
	}
	c.JSON(http.StatusCreated, publicAccount(a))
}

func (h *handler) login(c *gin.Context) {
	var in struct {
		StudentNo string `json:"studentNo"`
		Password  string `json:"password"`
	}
	if c.ShouldBindJSON(&in) != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请输入学号和密码")
		return
	}
	t, err := h.auth.Login(c, in.StudentNo, in.Password)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "学号或密码错误")
		return
	}
	h.setRefreshCookie(c, t)
	c.JSON(http.StatusOK, gin.H{"accessToken": t.AccessToken, "expiresAt": t.AccessExpiresAt})
}

func (h *handler) refresh(c *gin.Context) {
	raw, err := c.Cookie("refresh_token")
	if err != nil {
		writeError(c, http.StatusUnauthorized, "INVALID_SESSION", "登录状态已失效")
		return
	}
	t, err := h.auth.Refresh(c, raw)
	if err != nil {
		h.clearCookie(c)
		writeError(c, http.StatusUnauthorized, "INVALID_SESSION", "登录状态已失效")
		return
	}
	h.setRefreshCookie(c, t)
	c.JSON(http.StatusOK, gin.H{"accessToken": t.AccessToken, "expiresAt": t.AccessExpiresAt})
}

func (h *handler) logout(c *gin.Context) {
	raw, _ := c.Cookie("refresh_token")
	_ = h.auth.Logout(c, raw)
	h.clearCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *handler) me(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录")
		return
	}
	a, err := h.auth.AuthenticateAccessToken(c, strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效")
		return
	}
	c.JSON(http.StatusOK, publicAccount(a))
}

func (h *handler) setRefreshCookie(c *gin.Context, t auth.Tokens) {
	// 将刷新令牌限制在认证接口路径，并通过 HttpOnly 阻止 JavaScript 读取；
	// SameSite=Lax 用于降低常见跨站表单请求携带 Cookie 的风险。
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", t.RefreshToken, int(time.Until(t.RefreshExpiresAt).Seconds()), "/api/v1/auth", "", h.secureCookie, true)
}
func (h *handler) clearCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", "", -1, "/api/v1/auth", "", h.secureCookie, true)
}
func publicAccount(a auth.Account) gin.H {
	// 使用字段白名单构造响应，防止 Account 后续扩展时意外泄露 PasswordHash 等内部字段。
	return gin.H{"id": a.ID, "studentNo": a.StudentNo, "name": a.Name, "status": a.Status}
}
func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}
