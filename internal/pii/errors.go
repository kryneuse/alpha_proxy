package pii

import "errors"

var (
	ErrSessionNotFound      = errors.New("session not found")
	ErrProcessingInProgress = errors.New("processing in progress")
	ErrDetectorUnavailable  = errors.New("detector unavailable")
	ErrInvalidSpan          = errors.New("invalid span")
	ErrInvalidMLResponse    = errors.New("invalid ML response")
	ErrPolicyRejected       = errors.New("policy rejected")
	ErrPayloadIDConflict    = errors.New("payload id conflict")
	ErrStoreUnavailable     = errors.New("store unavailable")
	ErrDemaskingDisabled    = errors.New("demasking disabled")
	ErrUnknownToken         = errors.New("unknown token")
	ErrPayloadTooLarge      = errors.New("payload too large")
	ErrInvalidMaskKind      = errors.New("invalid mask kind")
)
