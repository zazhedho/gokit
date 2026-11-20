package binding

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestValidateUUIDKeepsHTTPValidationResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name      string
		value     string
		wantError string
	}{
		{name: "missing", wantError: "ID parameter is required"},
		{name: "invalid", value: "not-a-uuid", wantError: "ID must be a valid UUID"},
		{name: "valid", value: uuid.NewString()},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			if test.value != "" {
				ctx.Params = gin.Params{{Key: "id", Value: test.value}}
			}

			got, err := ValidateUUIDParam(ctx, uuid.New())
			if test.wantError == "" {
				requireID(t, test.value, got, err)
				return
			}
			requireBadRequest(t, recorder, test.wantError, err)
		})
	}
}

func requireID(t *testing.T, want, got string, err error) {
	t.Helper()
	if err != nil || got != want {
		t.Fatalf("expected id %q, got %q, err=%v", want, got, err)
	}
}

func requireBadRequest(t *testing.T, recorder *httptest.ResponseRecorder, wantBody string, err error) {
	t.Helper()
	if err == nil || recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), wantBody) {
		t.Fatalf("expected HTTP 400 with %q, status=%d body=%s err=%v", wantBody, recorder.Code, recorder.Body.String(), err)
	}
}

func TestValidateUUIDParamReturnsCanonicalID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := uuid.New()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Params = gin.Params{{Key: "id", Value: strings.ToUpper(id.String())}}
	got, err := ValidateUUIDParam(ctx, uuid.New())
	if err != nil || got != id.String() {
		t.Fatalf("got %q, %v; want %q", got, err, id)
	}
}
