package app

import (
	"encoding/json"
	"errors"

	"github.com/myrheimb/pageroom/internal/gitsync"
)

// AppError is how errors reach the UI: a machine code to react to and a
// message written for people, never a stack trace or a secret.
type AppError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Path    string            `json:"path,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// FormatError becomes the cause of the rejected promise in the UI. Wails
// also sends err.Error() unredacted as the message, which the UI ignores.
func FormatError(err error) []byte {
	ae := AppError{Code: "error", Message: redactor.String(err.Error())}
	var ue *gitsync.UserError
	var own *AppError
	switch {
	case errors.As(err, &ue):
		ae = AppError{Code: ue.Code, Message: ue.Msg}
	case errors.As(err, &own):
		ae = *own
		ae.Message = redactor.String(ae.Message)
	}
	// Marshalling strings and string maps cannot fail.
	b, _ := json.Marshal(ae)
	return b
}

func (e *AppError) Error() string { return e.Message }
