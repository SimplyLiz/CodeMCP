package a2a

import "testing"

func TestValidTransitions(t *testing.T) {
	tests := []struct {
		from  TaskState
		to    TaskState
		valid bool
	}{
		// submitted ->
		{TaskStateSubmitted, TaskStateWorking, true},
		{TaskStateSubmitted, TaskStateRejected, true},
		{TaskStateSubmitted, TaskStateCanceled, true},
		{TaskStateSubmitted, TaskStateCompleted, false},
		{TaskStateSubmitted, TaskStateFailed, false},

		// working ->
		{TaskStateWorking, TaskStateCompleted, true},
		{TaskStateWorking, TaskStateFailed, true},
		{TaskStateWorking, TaskStateInputRequired, true},
		{TaskStateWorking, TaskStateAuthRequired, true},
		{TaskStateWorking, TaskStateCanceled, true},
		{TaskStateWorking, TaskStateSubmitted, false},

		// input-required ->
		{TaskStateInputRequired, TaskStateWorking, true},
		{TaskStateInputRequired, TaskStateCanceled, true},
		{TaskStateInputRequired, TaskStateFailed, true},
		{TaskStateInputRequired, TaskStateCompleted, false},

		// auth-required ->
		{TaskStateAuthRequired, TaskStateWorking, true},
		{TaskStateAuthRequired, TaskStateCanceled, true},
		{TaskStateAuthRequired, TaskStateFailed, true},
		{TaskStateAuthRequired, TaskStateCompleted, false},

		// terminal states have no outgoing transitions
		{TaskStateCompleted, TaskStateWorking, false},
		{TaskStateFailed, TaskStateWorking, false},
		{TaskStateCanceled, TaskStateWorking, false},
		{TaskStateRejected, TaskStateWorking, false},
	}

	for _, tt := range tests {
		result := ValidTransition(tt.from, tt.to)
		if result != tt.valid {
			t.Errorf("ValidTransition(%s, %s) = %v, want %v", tt.from, tt.to, result, tt.valid)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []TaskState{TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateRejected}
	nonTerminal := []TaskState{TaskStateSubmitted, TaskStateWorking, TaskStateInputRequired, TaskStateAuthRequired}

	for _, s := range terminal {
		if !IsTerminal(s) {
			t.Errorf("IsTerminal(%s) = false, want true", s)
		}
	}
	for _, s := range nonTerminal {
		if IsTerminal(s) {
			t.Errorf("IsTerminal(%s) = true, want false", s)
		}
	}
}

func TestCanCancel(t *testing.T) {
	cancelable := []TaskState{TaskStateSubmitted, TaskStateWorking, TaskStateInputRequired, TaskStateAuthRequired}
	notCancelable := []TaskState{TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateRejected}

	for _, s := range cancelable {
		if !CanCancel(s) {
			t.Errorf("CanCancel(%s) = false, want true", s)
		}
	}
	for _, s := range notCancelable {
		if CanCancel(s) {
			t.Errorf("CanCancel(%s) = true, want false", s)
		}
	}
}
