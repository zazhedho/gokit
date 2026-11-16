package middlewares

import (
	"fmt"
	"github.com/zazhedho/gokit/response"
	"github.com/zazhedho/gokit/utils"
	"github.com/zazhedho/logus"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ErrorHandler(c *gin.Context, err any) {
	logId := utils.GenerateLogId(c)
	logus.WriteLogWithContext(c, logus.LogLevelPanic, fmt.Sprintf("RECOVERY; Error: %+v;", err))

	res := response.InternalServerError(logId)
	c.AbortWithStatusJSON(http.StatusInternalServerError, res)
}
