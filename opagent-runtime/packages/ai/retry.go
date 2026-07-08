package ai

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	retryStatusEqualsPattern     = regexp.MustCompile(`(?i)\bstatus(?:\s+code)?=?\s*(\d{3})\b`)
	retryHTTPStatusTextPattern   = regexp.MustCompile(`(?i)\b(\d{3})\s+(bad request|unauthorized|forbidden|not found|request timeout|conflict|too many requests|internal server error|bad gateway|service unavailable|gateway timeout)\b`)
	retryStandaloneStatusPattern = regexp.MustCompile(`\b(400|401|403|404|408|409|422|429|500|502|503|504)\b`)
)

// RetryError is the canonical transient-failure shape used across provider,
// runtime, and UI layers. It preserves the original error while surfacing the
// retry semantics the runtime needs.
type RetryError struct {
	Retryable    bool
	StatusCode   int
	Code         string
	Message      string
	RetryAfterMs int64
	Err          error
}

func (e *RetryError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return strings.TrimSpace(e.Message)
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "unknown error"
}

func (e *RetryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func AsRetryError(err error) (*RetryError, bool) {
	if err == nil {
		return nil, false
	}
	var retryErr *RetryError
	if !errors.As(err, &retryErr) || retryErr == nil {
		return nil, false
	}
	return retryErr, true
}

func WrapRetryError(err error, statusCode int, code, message string, retryAfterMs int64) error {
	if err == nil {
		return nil
	}
	normalized := NormalizeRetryError(err)
	if normalized == nil {
		return err
	}
	if statusCode > 0 {
		normalized.StatusCode = statusCode
	}
	if trimmedCode := strings.TrimSpace(code); trimmedCode != "" {
		normalized.Code = trimmedCode
	}
	if trimmedMessage := strings.TrimSpace(message); trimmedMessage != "" {
		normalized.Message = trimmedMessage
	}
	if retryAfterMs > 0 {
		normalized.RetryAfterMs = retryAfterMs
	}
	normalized.Retryable = isRetryableFailure(normalized.StatusCode, normalized.Code, normalized.Message)
	return normalized
}

func NormalizeRetryError(err error) *RetryError {
	if err == nil {
		return nil
	}
	if existing, ok := AsRetryError(err); ok {
		cloned := *existing
		if strings.TrimSpace(cloned.Message) == "" && cloned.Err != nil {
			cloned.Message = strings.TrimSpace(cloned.Err.Error())
		}
		cloned.Code = strings.TrimSpace(cloned.Code)
		cloned.Retryable = isRetryableFailure(cloned.StatusCode, cloned.Code, cloned.Message)
		return &cloned
	}

	message := strings.TrimSpace(err.Error())
	retryErr := &RetryError{
		Message: message,
		Err:     err,
	}
	if errors.Is(err, context.Canceled) {
		retryErr.Message = firstNonEmptyString(message, "retry cancelled")
		return retryErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		retryErr.Retryable = true
		retryErr.Message = firstNonEmptyString(message, "deadline exceeded")
		return retryErr
	}
	retryErr.StatusCode = extractRetryStatusCode(message)
	retryErr.Code = extractRetryCode(message)
	retryErr.Retryable = isRetryableFailure(retryErr.StatusCode, retryErr.Code, retryErr.Message)
	return retryErr
}

func IsRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func ParseRetryAfterHeaders(headers http.Header) int64 {
	if len(headers) == 0 {
		return 0
	}
	if raw := strings.TrimSpace(headers.Get("Retry-After-Ms")); raw != "" {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil && value > 0 {
			return value
		}
	}
	if raw := strings.TrimSpace(headers.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds > 0 {
			return int64(seconds * 1000)
		}
		if at, err := http.ParseTime(raw); err == nil {
			delay := time.Until(at)
			if delay > 0 {
				return delay.Milliseconds()
			}
		}
	}
	return 0
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func extractRetryStatusCode(message string) int {
	normalized := strings.TrimSpace(message)
	if normalized == "" {
		return 0
	}
	if matches := retryStatusEqualsPattern.FindStringSubmatch(normalized); len(matches) == 2 {
		if code, err := strconv.Atoi(matches[1]); err == nil && code >= 100 && code <= 599 {
			return code
		}
	}
	if matches := retryHTTPStatusTextPattern.FindStringSubmatch(normalized); len(matches) == 3 {
		if code, err := strconv.Atoi(matches[1]); err == nil && code >= 100 && code <= 599 {
			return code
		}
	}
	if matches := retryStandaloneStatusPattern.FindStringSubmatch(normalized); len(matches) == 2 {
		if code, err := strconv.Atoi(matches[1]); err == nil && code >= 100 && code <= 599 {
			return code
		}
	}
	return 0
}

func extractRetryCode(message string) string {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return ""
	}
	candidates := []string{
		"server_error",
		"overloaded_error",
		"overloaded",
		"rate_limit_exceeded",
		"rate_limit_error",
		"rate_limit",
		"upstream_error",
		"service_unavailable",
		"temporarily_unavailable",
		"timeout",
		"invalid_request_error",
		"context_length_exceeded",
	}
	for _, candidate := range candidates {
		if strings.Contains(normalized, candidate) {
			return candidate
		}
	}
	return ""
}

func isRetryableFailure(statusCode int, code, message string) bool {
	normalizedCode := strings.ToLower(strings.TrimSpace(code))
	normalizedMessage := strings.ToLower(strings.TrimSpace(message))
	if normalizedMessage == "" && statusCode == 0 && normalizedCode == "" {
		return false
	}
	if normalizedMessage == "retry cancelled" {
		return false
	}
	if isNonRetryableFailure(statusCode, normalizedCode, normalizedMessage) {
		return false
	}
	if IsRetryableStatusCode(statusCode) {
		return true
	}
	switch normalizedCode {
	case "server_error",
		"overloaded",
		"overloaded_error",
		"rate_limit",
		"rate_limit_error",
		"rate_limit_exceeded",
		"upstream_error",
		"service_unavailable",
		"temporarily_unavailable",
		"timeout":
		return true
	}
	retryableFragments := []string{
		"response failed without details",
		"server error",
		"overloaded",
		"rate limit",
		"too many requests",
		"service unavailable",
		"gateway timeout",
		"bad gateway",
		"upstream error",
		"upstream connect",
		"unexpected end of json input",
		"unexpected eof",
		"unexpected close",
		"connection reset",
		"connection refused",
		"closed network connection",
		"socket hang up",
		"timed out",
		"timeout",
		"i/o timeout",
		"eof",
		"abnormal closure",
		"websocket",
		"cloudflare",
		"terminated",
	}
	for _, fragment := range retryableFragments {
		if strings.Contains(normalizedMessage, fragment) {
			return true
		}
	}
	if normalizedMessage == "response failed" || strings.HasPrefix(normalizedMessage, "response failed:") {
		return true
	}
	return false
}

func isNonRetryableFailure(statusCode int, code, message string) bool {
	if statusCode > 0 && statusCode < 500 && !IsRetryableStatusCode(statusCode) {
		return true
	}
	switch code {
	case "invalid_request_error", "context_length_exceeded":
		return true
	}
	nonRetryableFragments := []string{
		"invalid 'input",
		"invalid input",
		"invalid_request_error",
		"invalid tool arguments",
		"schema",
		"system messages are not allowed",
		"threadid is required",
		"chatpath is required",
		"meta.agentid is required",
		"thread requires continuation",
		"thread has an incomplete turn",
		"replay item",
		"previous_response_id",
		"max_output_tokens",
		"context length",
		"context window",
		"compaction summary is unavailable",
	}
	for _, fragment := range nonRetryableFragments {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}
