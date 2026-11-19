// Package response builds the JSON envelopes returned by gokit handlers.
package response

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/google/uuid"

	"github.com/zazhedho/gokit/internal/messages"
)

// ErrorDetail is one entry of an envelope's error list.
type ErrorDetail struct {
	// Field is the request field the error refers to, when there is one.
	Field string `json:"field,omitempty"`
	// Code is a short machine-readable code such as "required" or "not_found".
	Code string `json:"code"`
	// Message is safe to show to the caller.
	Message string `json:"message"`
}

// APIResponse is the standard JSON envelope. Error is always a list of
// ErrorDetail and is omitted on success.
type APIResponse struct {
	LogID   uuid.UUID     `json:"log_id"`
	Code    int           `json:"code,omitempty"`
	Status  bool          `json:"status"`
	Message string        `json:"message"`
	Data    any           `json:"data,omitempty"`
	Error   []ErrorDetail `json:"error,omitempty"`
}

// PaginatedResponse is the JSON envelope for paginated lists.
type PaginatedResponse struct {
	LogID       uuid.UUID     `json:"log_id"`
	Code        int           `json:"code"`
	Status      bool          `json:"status"`
	Message     string        `json:"message"`
	TotalData   int64         `json:"total_data"`
	TotalPages  int64         `json:"total_pages"`
	CurrentPage int           `json:"current_page"`
	NextPage    bool          `json:"next_page"`
	PrevPage    bool          `json:"prev_page"`
	Limit       int           `json:"limit"`
	Data        any           `json:"data"`
	Error       []ErrorDetail `json:"error,omitempty"`
}

func isSuccess(code int) bool { return code >= http.StatusOK && code < http.StatusMultipleChoices }

// errorMessage returns msg, or the status text for code when msg is empty.
func errorMessage(code int, msg string) string {
	if msg != "" {
		return msg
	}
	if text := http.StatusText(code); text != "" {
		return text
	}
	return messages.MsgSomethingWrong
}

// errorCode derives a machine code such as "not_found" from an HTTP status.
func errorCode(code int) string {
	text := http.StatusText(code)
	if text == "" {
		return "error"
	}
	return strings.ReplaceAll(strings.ToLower(text), " ", "_")
}

// emptyIfNil replaces nil slices (typed or not) with an empty slice so they
// marshal as [] instead of null.
func emptyIfNil(data any) any {
	if data == nil {
		return nil
	}
	if rv := reflect.ValueOf(data); rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return data
}

// Response builds an envelope. For a 2xx code, msg is the message and data the
// payload. For any other code, msg (or the status text) is both the message
// and the single error detail.
func Response(code int, msg string, logID uuid.UUID, data any) *APIResponse {
	res := &APIResponse{LogID: logID, Code: code, Status: isSuccess(code), Data: emptyIfNil(data)}
	if res.Status {
		res.Message = msg
		return res
	}
	res.Message = errorMessage(code, msg)
	res.Error = []ErrorDetail{{Code: errorCode(code), Message: res.Message}}
	return res
}

// ErrorResponse builds an error envelope: msg is the message and publicError
// the caller-facing detail.
func ErrorResponse(code int, msg string, logID uuid.UUID, publicError string) *APIResponse {
	res := &APIResponse{LogID: logID, Code: code, Message: errorMessage(code, msg)}
	if publicError == "" {
		publicError = res.Message
	}
	res.Error = []ErrorDetail{{Code: errorCode(code), Message: publicError}}
	return res
}

// InternalServerError builds a 500 envelope that only references logID.
func InternalServerError(logID uuid.UUID) *APIResponse {
	return ErrorResponse(http.StatusInternalServerError, messages.MsgSomethingWrong, logID, fmt.Sprintf(messages.MsgInternal, logID.String()))
}

// Unauthorized builds a 401 envelope.
func Unauthorized(logID uuid.UUID, publicError string) *APIResponse {
	return ErrorResponse(http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized), logID, publicError)
}

// Forbidden builds a 403 envelope.
func Forbidden(logID uuid.UUID, publicError string) *APIResponse {
	return ErrorResponse(http.StatusForbidden, http.StatusText(http.StatusForbidden), logID, publicError)
}

// PaginationResponse builds a paginated envelope. An empty page keeps the
// status of code and carries the not-found message.
func PaginationResponse(code int, total int64, page, perPage int, logID uuid.UUID, data any) *PaginatedResponse {
	var totalPages int64
	switch {
	case total > 0 && perPage > 0:
		totalPages = total / int64(perPage)
		if total%int64(perPage) != 0 {
			totalPages++
		}
	case total > 0:
		totalPages = 1
	}

	message := messages.MsgSuccess
	if total == 0 || int64(page) > totalPages {
		message = messages.MsgNotFound
	}
	if data = emptyIfNil(data); data == nil {
		data = []any{}
	}

	return &PaginatedResponse{
		LogID:       logID,
		Code:        code,
		Status:      isSuccess(code),
		Message:     message,
		TotalData:   total,
		TotalPages:  totalPages,
		CurrentPage: page,
		NextPage:    int64(page) < totalPages,
		PrevPage:    page > 1,
		Limit:       perPage,
		Data:        data,
	}
}
