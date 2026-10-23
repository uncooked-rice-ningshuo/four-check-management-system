package workflow

import (
	"errors"
	"testing"
)

func TestSubmissionStateMachine(t *testing.T) {
	for _, tt := range []struct {
		from   State
		action Action
		want   State
	}{
		{Draft, Submit, PendingReview}, {Returned, Submit, PendingReview},
		{PendingReview, Return, Returned}, {PendingReview, Confirm, Confirmed},
	} {
		t.Run(string(tt.from)+"/"+string(tt.action), func(t *testing.T) {
			got, err := Transition(tt.from, tt.action)
			if err != nil || got != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
	for _, state := range []State{Draft, PendingReview, Returned, Confirmed, "LOCKED", "unknown"} {
		for _, action := range []Action{Submit, Return, Confirm, "EDIT", "LOCK", "unknown"} {
			allowed := (state == Draft || state == Returned) && action == Submit || state == PendingReview && (action == Return || action == Confirm)
			if allowed {
				continue
			}
			if _, err := Transition(state, action); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("accepted %s/%s: %v", state, action, err)
			}
		}
	}
}
