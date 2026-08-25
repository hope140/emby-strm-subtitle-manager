package d2

import "fmt"

// Error is a safe, stable D2 error. It intentionally contains no upstream
// body, URL, provider ID, path, token or exception text.
type Error struct {
	Status     int
	Code       string
	Message    string
	RetryAfter int
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func failure(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func rateLimitError() *Error {
	return &Error{Status: 429, Code: "rate_limited", Message: "request rate limit reached", RetryAfter: 1}
}
