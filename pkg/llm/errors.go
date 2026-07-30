package llm

import "errors"

var (
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrInvalidAPIKey       = errors.New("invalid API key")
	ErrTimeout             = errors.New("request timeout")
)
