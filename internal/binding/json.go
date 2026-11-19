package binding

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/zazhedho/gokit/internal/messages"
	"github.com/zazhedho/gokit/internal/response"
	"github.com/zazhedho/gokit/internal/utils"
	"github.com/zazhedho/logus"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func BindJSON[T any](ctx *gin.Context, logID uuid.UUID, logPrefix string, req *T) bool {
	if err := ctx.BindJSON(req); err != nil {
		logus.LogContext(ctx, logus.LevelError, fmt.Sprintf("%s; BindJSON ERROR: %s;", logPrefix, err.Error()))
		res := response.Response(http.StatusBadRequest, messages.MsgInvalidRequest, logID, nil)
		res.Error = utils.ValidateError(err, reflect.TypeOf(*req), "json")
		ctx.JSON(http.StatusBadRequest, res)
		return false
	}
	return true
}
