package transport

import "fmt"

// ErrTransportMessageConversion is an error produced when the transport
// message can not be converted.
type ErrTransportMessageConversion struct {
	fatal     bool
	transport string
	message   string
}

// NewErrMessageEncodingUnknown makes a new ErrMessageEncodingUnknown.
func NewErrTransportMessageConversion(transport, message string, fatal bool) *ErrTransportMessageConversion {
	return &ErrTransportMessageConversion{
		transport: transport,
		message:   message,
		fatal:     fatal,
	}
}

// IsFatal reports if this error should be considered fatal.
func (e *ErrTransportMessageConversion) IsFatal() bool {
	return e.fatal
}

// Error implements error.Error
func (e *ErrTransportMessageConversion) Error() string {
	return fmt.Sprintf("transport %s failed to convert message: %s", e.transport, e.message)
}
