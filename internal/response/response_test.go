package response

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func marshal(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestResponseSetsStatusFromCode(t *testing.T) {
	logID := uuid.New()

	success := Response(http.StatusCreated, "created", logID, map[string]string{"id": "1"})
	if !success.Status || success.LogID != logID || success.Message != "created" || success.Code != http.StatusCreated {
		t.Fatalf("unexpected success response: %+v", success)
	}

	failed := Response(http.StatusBadRequest, "bad input", logID, nil)
	if failed.Status || failed.Message != "bad input" {
		t.Fatalf("message parameter must be used: %+v", failed)
	}
	if len(failed.Error) != 1 || failed.Error[0].Code != "bad_request" || failed.Error[0].Message != "bad input" {
		t.Fatalf("unexpected error list: %+v", failed.Error)
	}

	if got := Response(http.StatusNotFound, "", logID, nil); got.Message != "Not Found" {
		t.Fatalf("empty message falls back to status text, got %q", got.Message)
	}
}

func TestErrorHelpersUseOneErrorShape(t *testing.T) {
	logID := uuid.New()

	got := InternalServerError(logID)
	want := fmt.Sprintf("Something went wrong. Please contact support with the log ID: %s.", logID)
	if got.Status || got.Message != "Something went wrong" || len(got.Error) != 1 || got.Error[0].Message != want {
		t.Fatalf("unexpected 500: %+v", got)
	}

	for name, res := range map[string]*APIResponse{
		"unauthorized": Unauthorized(logID, "login required"),
		"forbidden":    Forbidden(logID, "denied"),
		"custom":       ErrorResponse(http.StatusConflict, "Conflict", logID, "taken"),
	} {
		body := marshal(t, res)
		list, ok := body["error"].([]any)
		if !ok || len(list) != 1 {
			t.Fatalf("%s: error must be a list, got %#v", name, body["error"])
		}
		item := list[0].(map[string]any)
		if item["code"] == "" || item["message"] == "" {
			t.Fatalf("%s: item needs code and message: %#v", name, item)
		}
	}
	if res := ErrorResponse(http.StatusConflict, "Conflict", logID, "taken"); res.Message != "Conflict" || res.Error[0].Message != "taken" {
		t.Fatalf("msg and publicError must both be used: %+v", res)
	}
}

func TestLogIDIsAStringInEveryEnvelope(t *testing.T) {
	id := uuid.New()
	for name, v := range map[string]any{
		"api":       Response(http.StatusOK, "ok", id, nil),
		"paginated": PaginationResponse(http.StatusOK, 1, 1, 10, id, []int{1}),
	} {
		if got := marshal(t, v)["log_id"]; got != id.String() {
			t.Fatalf("%s: log_id = %#v", name, got)
		}
	}
}

func TestNilSlicesMarshalAsEmptyArrays(t *testing.T) {
	var nilItems []string
	api := marshal(t, Response(http.StatusOK, "ok", uuid.New(), nilItems))
	if got, ok := api["data"].([]any); !ok || len(got) != 0 {
		t.Fatalf("api data = %#v, want []", api["data"])
	}
	page := marshal(t, PaginationResponse(http.StatusOK, 0, 1, 10, uuid.New(), nilItems))
	if got, ok := page["data"].([]any); !ok || len(got) != 0 {
		t.Fatalf("paginated data = %#v, want []", page["data"])
	}
	page = marshal(t, PaginationResponse(http.StatusOK, 0, 1, 10, uuid.New(), nil))
	if got, ok := page["data"].([]any); !ok || len(got) != 0 {
		t.Fatalf("untyped nil data = %#v, want []", page["data"])
	}
}

func TestPaginationResponseCalculatesPageState(t *testing.T) {
	got := PaginationResponse(http.StatusOK, 55, 2, 20, uuid.New(), []string{"item"})

	if got.TotalPages != 3 || !got.NextPage || !got.PrevPage {
		t.Fatalf("unexpected page state: %+v", got)
	}
	if got.Limit != 20 || got.TotalData != 55 || got.Message != "Success" {
		t.Fatalf("unexpected pagination metadata: %+v", got)
	}
	var big int64 = 1 << 40
	if got := PaginationResponse(http.StatusOK, big, 1, 10, uuid.New(), []int{}); got.TotalData != big {
		t.Fatalf("total must be int64, got %d", got.TotalData)
	}
}

func TestEmptyPageIsStillSuccessful(t *testing.T) {
	for name, res := range map[string]*PaginatedResponse{
		"no rows":   PaginationResponse(http.StatusOK, 0, 1, 10, uuid.New(), []int{}),
		"past last": PaginationResponse(http.StatusOK, 5, 9, 10, uuid.New(), []int{}),
	} {
		if !res.Status || res.Message != "Data Not Found" {
			t.Fatalf("%s: want status true with the not-found message, got %+v", name, res)
		}
	}
}
