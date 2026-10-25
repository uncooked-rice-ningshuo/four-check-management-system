package httpapi

import (
	"github.com/gin-gonic/gin"
	"strings"
)

// Existing attendance URLs retain their type boundary; the course endpoint
// supports exactly the two approved course templates, never arbitrary schemas.
func courseTemplateAllowed(c *gin.Context, code string) bool {
	if strings.Contains(c.FullPath(), "/imports/attendance") {
		return code == "ATTENDANCE_IMPORT"
	}
	if strings.Contains(c.FullPath(), "/imports/inspections") && (code == "MORNING_ATTENDANCE_IMPORT" || code == "LATE_RETURN_IMPORT" || code == "DORMITORY_HYGIENE_IMPORT" || code == "CLASSROOM_HYGIENE_IMPORT" || code == "EVENING_SELF_STUDY_IMPORT") {
		return true
	}
	return code == "ATTENDANCE_IMPORT" || code == "UNATTENDED_CLASSROOM_IMPORT"
}
