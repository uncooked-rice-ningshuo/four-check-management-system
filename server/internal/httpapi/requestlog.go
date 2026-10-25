package httpapi

import (
	"log"
	"time"

	"four-check-management-system/server/internal/audit"
	"github.com/gin-gonic/gin"
)

// requestLogMiddleware logs only route templates and controlled metadata; URLs,
// headers, bodies and error messages may contain credentials or personal data.
func requestLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		logRequestStage(c, "request", "started")
		c.Next()
		metadata, _ := c.Get(requestMetadataKey)
		request, _ := metadata.(audit.Metadata)
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		log.Printf("api request completed: requestId=%s method=%s route=%s status=%d durationMs=%d aborted=%t", request.RequestID, c.Request.Method, route, c.Writer.Status(), time.Since(started).Milliseconds(), c.IsAborted())
	}
}

func logRequestStage(c *gin.Context, stage, result string) {
	metadata, _ := c.Get(requestMetadataKey)
	request, _ := metadata.(audit.Metadata)
	route := c.FullPath()
	if route == "" {
		route = "unmatched"
	}
	log.Printf("api request stage: requestId=%s method=%s route=%s stage=%s result=%s", request.RequestID, c.Request.Method, route, stage, result)
}
