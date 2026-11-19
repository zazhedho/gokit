package middleware

import (
	"fmt"
	"net/http"

	"github.com/zazhedho/gokit/internal/response"
	"github.com/zazhedho/gokit/internal/utils"
	"github.com/zazhedho/logus"

	"github.com/gin-gonic/gin"
)

func ErrorHandler(c *gin.Context, err any) {
	logId := utils.GenerateLogId(c)
	logus.LogContext(c, logus.LevelError, fmt.Sprintf("RECOVERY; Error: %+v;", err))

	res := response.InternalServerError(logId)
	c.AbortWithStatusJSON(http.StatusInternalServerError, res)
}
