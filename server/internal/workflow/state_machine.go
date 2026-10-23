// Package workflow owns department submission transitions and their invariants.
package workflow

import "errors"

// State is the persisted lifecycle of a department submission.
type State string

const (
	Draft         State = "DRAFT"
	PendingReview State = "PENDING_REVIEW"
	Returned      State = "RETURNED"
	Confirmed     State = "CONFIRMED"
)

// Action is an explicit use case; callers cannot assign arbitrary target states.
type Action string

const (
	Submit  Action = "SUBMIT"
	Return  Action = "RETURN"
	Confirm Action = "CONFIRM"
)

// ErrInvalidTransition prevents reopening confirmed submissions through ordinary workflow APIs.
var ErrInvalidTransition = errors.New("invalid submission transition")

// Transition validates lifecycle rules. Authorization, versions and record locks
// are additionally checked by the application service within its transaction.
func Transition(from State, action Action) (State, error) {
	switch {
	case (from == Draft || from == Returned) && action == Submit:
		return PendingReview, nil
	case from == PendingReview && action == Return:
		return Returned, nil
	case from == PendingReview && action == Confirm:
		return Confirmed, nil
	default:
		return "", ErrInvalidTransition
	}
}
