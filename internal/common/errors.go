package common

import "net/http"

// HTTPError carries an HTTP status with a message; Body (when set) replaces the message for proxy pass-through.
type HTTPError struct {
	Status  int
	Message string
	Body    any
}

func (e *HTTPError) Error() string { return e.Message }

func newErr(status int, msg string) *HTTPError { return &HTTPError{Status: status, Message: msg} }

// BadRequest → 400.
func BadRequest(msg string) *HTTPError { return newErr(http.StatusBadRequest, msg) }

// NotFound400 mirrors app-backend's generateNotFoundErrorResponse which returns 400.
func NotFound400(msg string) *HTTPError { return newErr(http.StatusBadRequest, msg) }

// NotFound → 404.
func NotFound(msg string) *HTTPError { return newErr(http.StatusNotFound, msg) }

// Gone → 410.
func Gone(msg string) *HTTPError { return newErr(http.StatusGone, msg) }

// Forbidden → 403.
func Forbidden(msg string) *HTTPError { return newErr(http.StatusForbidden, msg) }

// Unauthorized → 401.
func Unauthorized(msg string) *HTTPError { return newErr(http.StatusUnauthorized, msg) }

// Internal → 500.
func Internal(msg string) *HTTPError { return newErr(http.StatusInternalServerError, msg) }

// WithStatus builds an error with an arbitrary status.
func WithStatus(status int, msg string) *HTTPError { return newErr(status, msg) }

// StatusOf returns the HTTP status for err (500 when not an HTTPError).
func StatusOf(err error) int {
	if e, ok := err.(*HTTPError); ok && e.Status >= 100 && e.Status <= 599 {
		return e.Status
	}
	return http.StatusInternalServerError
}

// UnexpectedErrorMessage is the api-server fallback text.
const UnexpectedErrorMessage = "Unexpected error occurred"

// MessageOf returns the client-visible message for err.
func MessageOf(err error) string {
	if e, ok := err.(*HTTPError); ok && e.Message != "" {
		return e.Message
	}
	return UnexpectedErrorMessage
}
