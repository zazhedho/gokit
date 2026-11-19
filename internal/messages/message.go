// Package messages holds the standard response messages.
package messages

// Standard response messages.
const (
	// MsgInvalidRequest is returned when a request body cannot be bound or validated.
	MsgInvalidRequest = "Invalid request format. Please ensure the structure is correct and matches the expected data format."
	// MsgSomethingWrong is a generic failure message.
	MsgSomethingWrong = "Something went wrong"
	// MsgInternal is a format string taking the log ID.
	MsgInternal = "Something went wrong. Please contact support with the log ID: %s."
	// MsgExists is returned when a resource already exists.
	MsgExists = "Already exists."
	// MsgNotFound is returned when data is not found, and for an empty page.
	MsgNotFound = "Data Not Found"
	// MsgResourceNotFound is returned when a requested resource does not exist.
	MsgResourceNotFound = "The requested resource could not be found"
	// MsgSuccess is the generic success message.
	MsgSuccess = "Success"
	// MsgRequestTooLarge is returned when a request body exceeds the limit.
	MsgRequestTooLarge = "Request body too large"
	// MsgAccessDenied is returned when the caller lacks permission.
	MsgAccessDenied = "Access denied. You do not have the required permissions."
)
