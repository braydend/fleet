package ui

// newSessionForm collects the fields for a new session.
type newSessionForm struct {
	projectName string
	sessionName string
	branch      string
	base        string
	field       int // which input is focused
}

func (f newSessionForm) view() string {
	return "new session form (implemented in Task 12)\n"
}
