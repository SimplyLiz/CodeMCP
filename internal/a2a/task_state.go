package a2a

// validTransitions defines the allowed state transitions per the A2A spec.
var validTransitions = map[TaskState][]TaskState{
	TaskStateSubmitted:     {TaskStateWorking, TaskStateRejected, TaskStateCanceled},
	TaskStateWorking:       {TaskStateCompleted, TaskStateFailed, TaskStateInputRequired, TaskStateAuthRequired, TaskStateCanceled},
	TaskStateInputRequired: {TaskStateWorking, TaskStateCanceled, TaskStateFailed},
	TaskStateAuthRequired:  {TaskStateWorking, TaskStateCanceled, TaskStateFailed},
}

// terminalStates are states with no outgoing transitions.
var terminalStates = map[TaskState]bool{
	TaskStateCompleted: true,
	TaskStateFailed:    true,
	TaskStateCanceled:  true,
	TaskStateRejected:  true,
}

// ValidTransition returns true if transitioning from -> to is allowed.
func ValidTransition(from, to TaskState) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the state is a terminal (final) state.
func IsTerminal(state TaskState) bool {
	return terminalStates[state]
}

// CanCancel returns true if a task in the given state can be canceled.
func CanCancel(state TaskState) bool {
	return ValidTransition(state, TaskStateCanceled)
}
