package goheos

import (
	"errors"
	"fmt"
	"strconv"
)

// Sentinel errors. Use errors.Is to test for them.
var (
	// ErrNotConnected indicates a command was attempted while disconnected.
	ErrNotConnected = errors.New("goheos: not connected")
	// ErrDisconnected indicates the connection was lost while a command was
	// in flight.
	ErrDisconnected = errors.New("goheos: connection lost")
	// ErrInvalidArgument indicates a client-side validation failure.
	ErrInvalidArgument = errors.New("goheos: invalid argument")
	// ErrAuthenticationFailure matches CommandFailedError values whose error
	// codes indicate an authentication problem.
	ErrAuthenticationFailure = errors.New("goheos: authentication failure")
)

// ErrorID is an error code returned by the HEOS device on a failed command.
type ErrorID int

// Protocol error codes.
const (
	ErrorUnrecognizedCommand       ErrorID = 1
	ErrorInvalidID                 ErrorID = 2
	ErrorWrongArguments            ErrorID = 3
	ErrorDataNotAvailable          ErrorID = 4
	ErrorResourceNotAvailable      ErrorID = 5
	ErrorInvalidCredentials        ErrorID = 6
	ErrorCommandNotExecuted        ErrorID = 7
	ErrorUserNotLoggedIn           ErrorID = 8
	ErrorParameterOutOfRange       ErrorID = 9
	ErrorUserNotFound              ErrorID = 10
	ErrorInternal                  ErrorID = 11
	ErrorSystemError               ErrorID = 12
	ErrorProcessingPreviousCommand ErrorID = 13
	ErrorMediaCannotBePlayed       ErrorID = 14
	ErrorOptionNotSupported        ErrorID = 15
	ErrorTooManyCommandsInQueue    ErrorID = 16
	ErrorSkipLimitReached          ErrorID = 17
)

// Documented system error codes (syserrno of ErrorSystemError).
const (
	SystemErrorRemoteServiceError         = -9
	SystemErrorServiceNotRegistered       = -1061
	SystemErrorUserNotLoggedIn            = -1063
	SystemErrorUserNotFound               = -1056
	SystemErrorContentAuthenticationError = -1201
	SystemErrorContentAuthorizationError  = -1232
	SystemErrorAccountParametersInvalid   = -1239
)

// CommandError indicates a command could not be sent or no valid response was
// received (connection failure, timeout, cancellation). The underlying cause
// is available via Unwrap/errors.Is.
type CommandError struct {
	// Command is the protocol command name, e.g. "player/set_volume".
	Command string
	// Err is the underlying cause.
	Err error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("goheos: command %s: %v", e.Command, e.Err)
}

// Unwrap returns the underlying cause.
func (e *CommandError) Unwrap() error { return e.Err }

// CommandFailedError indicates the device processed a command and returned a
// failure response (result "fail").
type CommandFailedError struct {
	// Command is the protocol command name.
	Command string
	// ErrorID is the protocol error code.
	ErrorID ErrorID
	// ErrorText is the error description returned by the device. For
	// ErrorSystemError the system error number is appended.
	ErrorText string
	// SystemErrorNumber carries the syserrno value when ErrorID is
	// ErrorSystemError; zero otherwise.
	SystemErrorNumber int
}

func (e *CommandFailedError) Error() string {
	return fmt.Sprintf("goheos: command %s failed: %s (%d)", e.Command, e.ErrorText, e.ErrorID)
}

// Is reports ErrAuthenticationFailure for authentication-related error codes,
// replacing pyheos' CommandAuthenticationError subclass.
func (e *CommandFailedError) Is(target error) bool {
	if target == ErrAuthenticationFailure {
		return e.isAuthentication()
	}
	return false
}

func (e *CommandFailedError) isAuthentication() bool {
	if e.ErrorID == ErrorSystemError {
		return e.SystemErrorNumber == SystemErrorUserNotLoggedIn ||
			e.SystemErrorNumber == SystemErrorUserNotFound
	}
	return e.ErrorID == ErrorInvalidCredentials ||
		e.ErrorID == ErrorUserNotLoggedIn ||
		e.ErrorID == ErrorUserNotFound
}

// newCommandFailedError builds the error from a failure response message.
func newCommandFailedError(m *message) *CommandFailedError {
	text := m.message[attrText]
	errorID, _ := m.intValue(attrErrorID)
	e := &CommandFailedError{
		Command:   m.command,
		ErrorID:   ErrorID(errorID),
		ErrorText: text,
	}
	if e.ErrorID == ErrorSystemError {
		if n, err := m.intValue(attrSystemErrorNumber); err == nil {
			e.SystemErrorNumber = n
			e.ErrorText += " " + strconv.Itoa(n)
		}
	}
	return e
}
