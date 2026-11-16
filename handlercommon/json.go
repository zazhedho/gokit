package handlercommon

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/zazhedho/gokit/messages"
	"github.com/zazhedho/gokit/response"
	"github.com/zazhedho/gokit/utils"
	"github.com/zazhedho/logus"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func BindJSON[T any](ctx *gin.Context, logID uuid.UUID, logPrefix string, req *T) bool {
	if err := ctx.BindJSON(req); err != nil {
		logus.WriteLogWithContext(ctx, logus.LogLevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))
		res := response.Response(http.StatusBadRequest, messages.InvalidRequest, logID, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(*req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return false
	}
	return true
}
