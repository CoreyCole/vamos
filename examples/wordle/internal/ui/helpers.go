package ui

import "fmt"

func tileClass(state string) string {
	base := "grid size-14 place-items-center rounded-xl border text-2xl font-black uppercase sm:size-16 sm:text-3xl"
	switch state {
	case "green":
		return base + " border-emerald-300 bg-emerald-500 text-white"
	case "yellow":
		return base + " border-amber-300 bg-amber-500 text-white"
	case "gray":
		return base + " border-slate-500 bg-slate-700 text-white"
	default:
		return base + " border-white/15 bg-slate-950/60 text-slate-400"
	}
}

func keyClass(state string) string {
	base := "rounded-lg px-2 py-2 text-xs font-black sm:px-3"
	switch state {
	case "green":
		return base + " bg-emerald-500 text-white"
	case "yellow":
		return base + " bg-amber-500 text-white"
	case "gray":
		return base + " bg-slate-700 text-slate-300"
	default:
		return base + " bg-white/10 text-slate-200"
	}
}

func guessCount(used, max int) string {
	return fmt.Sprintf("%d/%d", used, max)
}
