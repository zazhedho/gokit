package messages

const (
	MsgInvalidRequest   = "Invalid request format. Please ensure the structure is correct and matches the expected data format."
	MsgSomethingWrong   = "Something went wrong"
	MsgInternal         = "Something went wrong. Please contact support with the log ID: %s."
	MsgExists           = "Already exists."
	MsgNotFound         = "Data Not Found"
	MsgResourceNotFound = "The requested resource could not be found"
	MsgSuccess          = "Success"
	MsgRequestTooLarge  = "Request body too large"
	MsgAccessDenied     = "Access denied. You do not have the required permissions."
)
