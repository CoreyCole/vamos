package workbench

// threadsOpenAriaHidden is "true" when the left rail starts open (reopen slot invisible).
func threadsOpenAriaHidden(threadsOpen bool) string {
	if threadsOpen {
		return "true"
	}
	return "false"
}
