package binding

import (
	"errors"
	"net/http"

	"github.com/zazhedho/gokit/internal/response"
	"github.com/zazhedho/gokit/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ValidateUUIDParam validates the "id" route parameter and returns it in
// canonical form. On failure it writes a 400 envelope and returns the error.
func ValidateUUIDParam(ctx *gin.Context, logID uuid.UUID) (string, error) {
	id, err := utils.ValidateUUID(ctx.Param("id"))
	if err == nil {
		return id, nil
	}

	res := response.Response(http.StatusBadRequest, http.StatusText(http.StatusBadRequest), logID, nil)
	if errors.Is(err, utils.ErrMissingID) {
		res.Error = []response.ErrorDetail{{Field: "id", Code: "required", Message: "ID parameter is required"}}
	} else {
		res.Error = []response.ErrorDetail{{Field: "id", Code: "uuid", Message: "ID must be a valid UUID"}}
	}
	ctx.AbortWithStatusJSON(http.StatusBadRequest, res)
	return "", err
}
