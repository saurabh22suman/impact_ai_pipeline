package providers

import (
	"errors"
	"fmt"
)

var (
	ErrProviderConfig = errors.New("provider config error")
	ErrProviderAuth   = errors.New("provider auth error")
)

type fatalProviderError struct {
	cause error
}

func (e fatalProviderError) Error() string {
	return e.cause.Error()
}

func (e fatalProviderError) Unwrap() error {
	return e.cause
}

func NewFatalProviderError(cause error, message string) error {
	if cause == nil {
		cause = errors.New("fatal provider error")
	}
	if message == "" {
		return fatalProviderError{cause: cause}
	}
	return fatalProviderError{cause: fmt.Errorf("%s: %w", message, cause)}
}

func IsFatalProviderError(err error) bool {
	var target fatalProviderError
	return errors.As(err, &target)
}
