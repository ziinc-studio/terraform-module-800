package client

import (
	"encoding/json"
	"fmt"
)

// APIError is the typed error returned by the client for any non-2xx
// response. Resource code branches on StatusCode (e.g., 404 -> remove
// from state, 403 -> diagnostic about scope).
type APIError struct {
	StatusCode int
	Message    string
	Errors     []string
	RawBody    []byte
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("800.com API: %d %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("800.com API: %d", e.StatusCode)
}

// IsNotFound reports whether the error represents a 404. Resource Read
// methods use this to decide between "drift, remove from state" and
// "actual error, surface to operator".
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if ae, ok := err.(*APIError); ok {
		return ae.StatusCode == 404
	}
	return false
}

// parseAPIError handles both observed error envelopes:
//
//	{"errors": ["..."], "message": "..."}        // 401, 403, 422
//	{"metadata": [], "data": {"message": "..."}} // 404
func parseAPIError(status int, raw []byte) error {
	out := &APIError{StatusCode: status, RawBody: raw}

	var shapeA struct {
		Errors  []string `json:"errors"`
		Message string   `json:"message"`
	}
	if err := json.Unmarshal(raw, &shapeA); err == nil && (shapeA.Message != "" || len(shapeA.Errors) > 0) {
		out.Message = shapeA.Message
		out.Errors = shapeA.Errors
		return out
	}

	var shapeB struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &shapeB); err == nil && shapeB.Data.Message != "" {
		out.Message = shapeB.Data.Message
		return out
	}

	out.Message = string(raw)
	return out
}
