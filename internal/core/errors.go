package core

import (
	"fmt"
	"strings"
)

// Error is a failure the user or an agent can act on.
//
// Every failure states what happened and the next command to run (OVERHAUL.md
// G7). Remediation is not decoration: it is the field that lets an agent
// recover on its own instead of giving up, so an Error without one is a bug.
type Error struct {
	// Code is stable and machine-facing. It appears in JSON output and must
	// not change once released.
	Code string
	// Message says what happened, in one sentence, without apology.
	Message string
	// Remediation is the exact command or action that fixes it.
	Remediation string
	// Exit is the process exit code for this class of failure.
	Exit int
	// Err is the underlying cause, kept for wrapping but never shown raw:
	// an SDK stack trace is not a road sign.
	Err error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// Error codes. Stable strings — part of the agent contract.
const (
	CodeNoCredentials = "no_credentials"
	CodeAuthExpired   = "auth_expired"
	CodePermission    = "permission_denied"
	CodeNotManagement = "not_management_account"
	CodeAWS           = "aws_error"
)

func errNoCredentials() *Error {
	return &Error{
		Code:        CodeNoCredentials,
		Message:     "no AWS credentials found",
		Remediation: "run 'aws sso login', or set AWS_PROFILE, or run 'finops auth' to choose a source",
		Exit:        3,
	}
}

func errAuthExpired(session string, ago string) *Error {
	return &Error{
		Code:        CodeAuthExpired,
		Message:     fmt.Sprintf("IAM Identity Center session %q expired %s ago", session, ago),
		Remediation: fmt.Sprintf("aws sso login --sso-session %s", session),
		Exit:        3,
	}
}

func errUnknownProfile(want string, available []string) *Error {
	e := &Error{
		Code:    CodeNoCredentials,
		Message: fmt.Sprintf("no AWS profile named %q", want),
		Exit:    3,
	}
	if len(available) == 0 {
		e.Remediation = "no profiles are configured — run 'aws configure sso' or 'aws configure'"
	} else {
		e.Remediation = fmt.Sprintf("available profiles: %s", strings.Join(available, ", "))
	}
	return e
}

func errAWS(action string, err error) *Error {
	// The underlying error is kept on Err for --debug and for errors.Is/As, but
	// deliberately not interpolated into Message. An SDK credential-chain
	// failure reads as a paragraph of provider internals ending in "no EC2 IMDS
	// role found": every word true, none of it actionable, and alarming to
	// someone who just wanted their bill.
	return &Error{
		Code:        CodeAWS,
		Message:     action + " failed",
		Remediation: "check the AWS status of this account, or re-run with --debug for the underlying error",
		Exit:        1,
		Err:         err,
	}
}

// Detail returns the underlying cause for --debug output. Callers must not put
// this in front of a user by default.
func (e *Error) Detail() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}
