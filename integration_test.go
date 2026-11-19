package gokit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zazhedho/gokit"
)

type createItem struct {
	Name string   `json:"name" binding:"required,min=3"`
	Tags []string `json:"tags"`
}

func newAPI() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gokit.RequestIDMiddleware(), gokit.Recovery(), gokit.CORS(gokit.CORSOptions{AllowOrigins: []string{"https://app.example"}}))

	r.POST("/items", func(c *gin.Context) {
		logID := gokit.RequestID(c)
		var req createItem
		if !gokit.BindJSON(c, logID, "[Items][Create]", &req, gokit.WithMaxBodyBytes(256)) {
			return
		}
		c.JSON(http.StatusCreated, gokit.Response(http.StatusCreated, gokit.MsgSuccess, logID, req))
	})
	r.GET("/items", func(c *gin.Context) {
		c.JSON(http.StatusOK, gokit.PaginationResponse(http.StatusOK, 0, 1, 10, gokit.RequestID(c), []createItem(nil)))
	})
	r.GET("/boom", func(*gin.Context) { panic("secret internals") })
	return r
}

func call(r *gin.Engine, method, path, body string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestEveryErrorUsesOneShapeAndTheRequestID(t *testing.T) {
	r := newAPI()
	id := uuid.NewString()
	hdr := map[string]string{"X-Request-ID": id}

	cases := map[string]struct {
		method, path, body string
		status             int
	}{
		"validation": {http.MethodPost, "/items", `{"name":"x"}`, http.StatusBadRequest},
		"type":       {http.MethodPost, "/items", `{"name":5}`, http.StatusBadRequest},
		"syntax":     {http.MethodPost, "/items", `{`, http.StatusBadRequest},
		"too large":  {http.MethodPost, "/items", `{"name":"` + strings.Repeat("a", 300) + `"}`, http.StatusRequestEntityTooLarge},
		"panic":      {http.MethodGet, "/boom", "", http.StatusInternalServerError},
	}
	for name, tc := range cases {
		rec, body := call(r, tc.method, tc.path, tc.body, hdr)
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, tc.status, rec.Body.String())
			continue
		}
		list, ok := body["error"].([]any)
		if !ok || len(list) == 0 {
			t.Errorf("%s: error must be a non-empty list: %s", name, rec.Body.String())
			continue
		}
		for _, item := range list {
			m := item.(map[string]any)
			if m["code"] == nil || m["message"] == nil {
				t.Errorf("%s: item needs code and message: %v", name, m)
			}
		}
		if body["log_id"] != id || rec.Header().Get("X-Request-ID") != id {
			t.Errorf("%s: log_id %v / header %q, want %s", name, body["log_id"], rec.Header().Get("X-Request-ID"), id)
		}
		if strings.Contains(rec.Body.String(), "secret internals") || strings.Contains(rec.Body.String(), "createItem") {
			t.Errorf("%s: body leaks internals: %s", name, rec.Body.String())
		}
	}
}

func TestEmptyPageHasEmptyArrayAndSuccessStatus(t *testing.T) {
	_, body := call(newAPI(), http.MethodGet, "/items", "", nil)
	if body["status"] != true || body["total_data"] != float64(0) {
		t.Fatalf("unexpected body: %v", body)
	}
	if data, ok := body["data"].([]any); !ok || len(data) != 0 {
		t.Fatalf("data must be [] for an empty page, got %#v", body["data"])
	}
}

func TestCORSAndExposedRequestID(t *testing.T) {
	r := newAPI()
	rec, _ := call(r, http.MethodGet, "/items", "", map[string]string{"Origin": "https://app.example"})
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" ||
		!strings.Contains(rec.Header().Get("Access-Control-Expose-Headers"), "X-Request-ID") {
		t.Fatalf("headers: %v", rec.Header())
	}
}
