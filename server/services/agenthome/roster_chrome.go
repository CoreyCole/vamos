package agenthome

func rosterIsPinned(kind RoomKind, id string) bool {
	return (kind == KindDM && id == "bot") || (kind == KindGroup && id == "vamos-dev")
}

func rosterAgentRowClass(selected bool) string {
	base := "roster-row flex items-start gap-2.5 rounded-lg px-1.5 py-2 transition-colors"
	if selected {
		return base + " roster-row-selected"
	}
	return base
}

func rosterPlanRowClass(selected bool) string {
	base := "roster-row roster-row-plan flex items-start gap-2.5 rounded-lg px-1.5 py-1.5 transition-colors"
	if selected {
		return base + " roster-row-selected"
	}
	return base
}

func rosterPinClass(selected bool) string {
	base := "roster-pin flex flex-col items-center gap-1 rounded-2xl px-1.5 py-1.5 text-center transition-colors"
	if selected {
		return base + " roster-pin-selected"
	}
	return base
}
