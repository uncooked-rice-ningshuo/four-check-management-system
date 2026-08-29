package httpapi

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const requestMetadataKey = "auditRequestMetadata"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// ConfigureTrustedProxies 只信任显式配置的代理网段；空配置表示不信任转发头。
func ConfigureTrustedProxies(router *gin.Engine, rawCIDRs string) error {
	var trusted []string
	for _, value := range strings.Split(rawCIDRs, ",") {
		if value = strings.TrimSpace(value); value != "" {
			trusted = append(trusted, value)
		}
	}
	return router.SetTrustedProxies(trusted)
}

func requestMetadataMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if !validRequestID.MatchString(requestID) {
			requestID = uuid.NewString()
		}
		metadata := audit.Metadata{
			RequestID: requestID,
			IP:        truncateRequestValue(c.ClientIP(), 64),
			UserAgent: truncateRequestValue(c.Request.UserAgent(), 512),
		}
		c.Set(requestMetadataKey, metadata)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// requestMetadata 从受控上下文构造审计元数据，不读取请求体、Cookie 或 Authorization。
func requestMetadata(c *gin.Context, reason string) audit.Metadata {
	metadata, _ := c.Get(requestMetadataKey)
	result, _ := metadata.(audit.Metadata)
	if account, ok := c.Get("account"); ok {
		if authenticated, valid := account.(auth.Account); valid {
			result.ActorID = authenticated.ID
		}
	}
	result.Reason = truncateRequestValue(strings.TrimSpace(reason), 255)
	return result
}

func truncateRequestValue(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
