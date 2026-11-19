package binding

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zazhedho/logus"

	"github.com/gin-gonic/gin"
)

type bindJSONTestRequest struct {
	Name string `json:"name" binding:"required"`
}

func TestBindJSONReturnsFalseAndWritesBadRequestOnInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	var req bindJSONTestRequest
	if BindJSON(ctx, uuid.New(), "[Test][BindJSON]", &req) {
		t.Fatal("expected BindJSON to return false")
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"message":"Invalid request format`) {
		t.Fatalf("expected invalid request response, got %s", rec.Body.String())
	}
}

func TestBindJSONReturnsTrueOnValidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"name":"starter"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	var req bindJSONTestRequest
	if !BindJSON(ctx, uuid.New(), "[Test][BindJSON]", &req) {
		t.Fatal("expected BindJSON to return true")
	}

	if req.Name != "starter" {
		t.Fatalf("expected name to be decoded, got %q", req.Name)
	}
}

func newBindContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, rec
}

func TestBindJSONRejectsOversizedBodyWith413(t *testing.T) {
	body := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	ctx, rec := newBindContext(body)

	var req bindJSONTestRequest
	if BindJSON(ctx, uuid.New(), "[Test]", &req) {
		t.Fatal("a body over the default limit must be rejected")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"error":[{`) {
		t.Fatalf("expected the error list shape, got %s", rec.Body.String())
	}
}

func TestBindJSONLimitIsConfigurable(t *testing.T) {
	ctx, rec := newBindContext(`{"name":"0123456789"}`)
	var req bindJSONTestRequest
	if BindJSON(ctx, uuid.New(), "[Test]", &req, WithMaxBodyBytes(8)) || rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 with an 8 byte limit, got %d", rec.Code)
	}

	ctx, _ = newBindContext(`{"name":"0123456789"}`)
	if !BindJSON(ctx, uuid.New(), "[Test]", &req, WithMaxBodyBytes(1024)) || req.Name != "0123456789" {
		t.Fatal("a body under the limit must bind")
	}
}

func TestBindJSONDoesNotEchoDecoderInternals(t *testing.T) {
	ctx, rec := newBindContext(`{"name":123}`)
	var req bindJSONTestRequest
	if BindJSON(ctx, uuid.New(), "[Test]", &req) || rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "bindJSONTestRequest") || strings.Contains(body, "Go struct") {
		t.Fatalf("body leaks Go names: %s", body)
	}
	if !strings.Contains(body, `"field":"name"`) || !strings.Contains(body, "invalid_type") {
		t.Fatalf("expected JSON field name and code, got %s", body)
	}

	ctx, rec = newBindContext(`{not json`)
	if BindJSON(ctx, uuid.New(), "[Test]", &req) || !strings.Contains(rec.Body.String(), "Malformed JSON") {
		t.Fatalf("expected the fixed syntax message, got %s", rec.Body.String())
	}
}

func TestBindJSONLogsFullErrorServerSide(t *testing.T) {
	buf := &bytes.Buffer{}
	logus.SetOutput(buf)
	t.Cleanup(func() { logus.SetOutput(nil) })

	ctx, _ := newBindContext(`{"name":123}`)
	var req bindJSONTestRequest
	BindJSON(ctx, uuid.New(), "[Test][Prefix]", &req)
	if !strings.Contains(buf.String(), "[Test][Prefix]") || !strings.Contains(buf.String(), "bindJSONTestRequest") {
		t.Fatalf("the full decoder error must be logged, got %s", buf.String())
	}
}
