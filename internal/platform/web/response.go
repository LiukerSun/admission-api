package web

// Response is the unified API response envelope.
type Response struct {
	Code    int    `json:"code"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

// ErrorResponse returns a simple error response.
func ErrorResponse(code int, message string) Response {
	return Response{
		Code:    code,
		Message: message,
	}
}

// SuccessResponse returns a success response with data.
func SuccessResponse(data any) Response {
	return Response{
		Code:    0,
		Data:    data,
		Message: "ok",
	}
}
