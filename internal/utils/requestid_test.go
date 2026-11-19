package utils

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zazhedho/logus"
)

func TestRequestIDIsSharedWithLogus(t *testing.T) {
	id := uuid.New()

	if got := logus.RequestID(WithRequestID(context.Background(), id)); got != id {
		t.Fatalf("logus did not read utils request id: got %s want %s", got, id)
	}
	if got := RequestID(logus.WithRequestID(context.Background(), id)); got != id {
		t.Fatalf("RequestID did not read logus request id: got %s want %s", got, id)
	}
}

func TestRequestIDOnGinContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	id := uuid.New()
	c.Set(logus.CtxKeyRequestID, id)
	if got := RequestID(c); got != id {
		t.Fatalf("expected stored id %s, got %s", id, got)
	}

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	generated := RequestID(c2)
	if generated == uuid.Nil || RequestID(c2) != generated {
		t.Fatalf("generated id must be stable on the same context, got %s then %s", generated, RequestID(c2))
	}
}
