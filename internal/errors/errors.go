// Package errors provides enhanced error handling utilities
package errors

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorType represents the type of error
type ErrorType string

const (
	ErrorTypeUnknown     ErrorType = "unknown"
	ErrorTypeNetwork     ErrorType = "network"
	ErrorTypeValidation  ErrorType = "validation"
	ErrorTypeIO          ErrorType = "io"
	ErrorTypeNotFound    ErrorType = "not_found"
	ErrorTypeTimeout     ErrorType = "timeout"
	ErrorTypeRateLimit   ErrorType = "rate_limit"
	ErrorTypePermission  ErrorType = "permission"
	ErrorTypeConfig      ErrorType = "config"
	ErrorTypeParse       ErrorType = "parse"
)

// AppError is an enhanced error type with additional context
type AppError struct {
	Type     ErrorType `json:"type"`
	Message  string    `json:"message"`
	Code     int       `json:"code,omitempty"`
	URL      string    `json:"url,omitempty"`
	Details  string    `json:"details,omitempty"`
	Cause    error     `json:"-"` // Excluded from JSON
}

// Error implements the error interface
func (e *AppError) Error() string {
	var sb strings.Builder
	
	// Add type
	sb.WriteString(string(e.Type))
	if e.Message != "" {
		sb.WriteString(": ")
		sb.WriteString(e.Message)
	}
	
	// Add details if present
	if e.Details != "" {
		sb.WriteString(" ("")
		sb.WriteString(e.Details)
		sb.WriteString(")")
	}
	
	// Add cause if present
	if e.Cause != nil {
		sb.WriteString(". Cause: ")
		sb.WriteString(e.Cause.Error())
	}
	
	return sb.String()
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// New creates a new AppError
func New(errorType ErrorType, message string) *AppError {
	return &AppError{
		Type:    errorType,
		Message: message,
	}
}

// Newf creates a new AppError with formatted message
func Newf(errorType ErrorType, format string, args ...interface{}) *AppError {
	return &AppError{
		Type:    errorType,
		Message: fmt.Sprintf(format, args...),
	}
}

// Wrap wraps an existing error with additional context
func Wrap(errorType ErrorType, err error, message string) *AppError {
	if err == nil {
		return nil
	}
	
	var appErr *AppError
	if errors.As(err, &appErr) {
		// Already an AppError, just update the message
		appErr.Message = message
		return appErr
	}
	
	return &AppError{
		Type:    errorType,
		Message: message,
		Cause:   err,
	}
}

// Wrapf wraps an existing error with formatted message
func Wrapf(errorType ErrorType, err error, format string, args ...interface{}) *AppError {
	if err == nil {
		return nil
	}
	
	return &AppError{
		Type:    errorType,
		Message: fmt.Sprintf(format, args...),
		Cause:   err,
	}
}

// WithCode adds an error code to an AppError
func (e *AppError) WithCode(code int) *AppError {
	e.Code = code
	return e
}

// WithURL adds a URL to an AppError
func (e *AppError) WithURL(url string) *AppError {
	e.URL = url
	return e
}

// WithDetails adds details to an AppError
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// Is checks if the error is of a specific type
func Is(err error, errorType ErrorType) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == errorType
	}
	return false
}

// Type returns the error type, or ErrorTypeUnknown if not an AppError
func Type(err error) ErrorType {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type
	}
	return ErrorTypeUnknown
}

// Retryable returns true if the error is retryable
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	
	switch Type(err) {
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeRateLimit:
		return true
	default:
		return false
	}
}

// Temporary returns true if the error is temporary (similar to net.Error.Temporary)
func Temporary(err error) bool {
	return Retryable(err)
}

// Common error constructors

// NetworkError creates a network-related error
func NetworkError(message string, err error) *AppError {
	return Wrap(ErrorTypeNetwork, err, message)
}

// ValidationError creates a validation error
func ValidationError(message string, err error) *AppError {
	return Wrap(ErrorTypeValidation, err, message)
}

// IOError creates an IO error
func IOError(message string, err error) *AppError {
	return Wrap(ErrorTypeIO, err, message)
}

// NotFoundError creates a not found error
func NotFoundError(resource string, id string) *AppError {
	return Newf(ErrorTypeNotFound, "%s '%s' not found", resource, id)
}

// TimeoutError creates a timeout error
func TimeoutError(message string, err error) *AppError {
	return Wrap(ErrorTypeTimeout, err, message)
}

// RateLimitError creates a rate limit error
func RateLimitError(message string, err error) *AppError {
	return Wrap(ErrorTypeRateLimit, err, message)
}

// PermissionError creates a permission error
func PermissionError(message string, err error) *AppError {
	return Wrap(ErrorTypePermission, err, message)
}

// ConfigError creates a configuration error
func ConfigError(message string, err error) *AppError {
	return Wrap(ErrorTypeConfig, err, message)
}

// ParseError creates a parse error
func ParseError(message string, err error) *AppError {
	return Wrap(ErrorTypeParse, err, message)
}

// ErrorChain represents a chain of errors
func ErrorChain(err error) []error {
	var chain []error
	for err != nil {
		chain = append(chain, err)
		err = errors.Unwrap(err)
	}
	return chain
}

// RootCause returns the root cause of an error
func RootCause(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}

// FormatError formats an error with its chain
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	
	var sb strings.Builder
	chain := ErrorChain(err)
	
	for i, e := range chain {
		if i > 0 {
			sb.WriteString(" -> ")
		}
		
		// Check if it's an AppError
		var appErr *AppError
		if errors.As(e, &appErr) {
			sb.WriteString(fmt.Sprintf("%s: %s", appErr.Type, appErr.Message))
			if appErr.Details != "" {
				sb.WriteString(" (")
				sb.WriteString(appErr.Details)
				sb.WriteString(")")
			}
		} else {
			sb.WriteString(e.Error())
		}
	}
	
	return sb.String()
}

// HTTPStatusFromError returns an appropriate HTTP status code for an error
func HTTPStatusFromError(err error) int {
	if err == nil {
		return 200
	}
	
	switch Type(err) {
	case ErrorTypeNotFound:
		return 404
	case ErrorTypeValidation:
		return 400
	case ErrorTypePermission:
		return 403
	case ErrorTypeRateLimit:
		return 429
	case ErrorTypeTimeout:
		return 504
	case ErrorTypeNetwork:
		return 502
	default:
		return 500
	}
}

// IsContextCanceled checks if the error is a context cancellation
func IsContextCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled")
}

// IsContextDeadlineExceeded checks if the error is a context deadline exceeded
func IsContextDeadlineExceeded(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded")
}
