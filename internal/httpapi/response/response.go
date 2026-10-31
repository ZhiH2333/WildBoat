package response

import (
	"encoding/json"
	"net/http"
)

// ErrorBody is the stable public error envelope.
type ErrorBody struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// Error writes a JSON error envelope.
func Error(w http.ResponseWriter, status int, code, message, requestID string, details map[string]any) {
	JSON(w, status, errorEnvelope{
		Error: ErrorBody{
			Code:      code,
			Message:   message,
			RequestID: requestID,
			Details:   details,
		},
	})
}
