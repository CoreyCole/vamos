package ui

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
)

func rowClass(row GuessRow) string {
	base := "grid grid-cols-5 gap-2"
	if row.Current {
		return base + " rounded-2xl"
	}
	return base
}

func tileClass(state string) string {
	base := "grid size-14 place-items-center rounded-xl border text-2xl font-black uppercase sm:size-16 sm:text-3xl"
	switch state {
	case "correct":
		return base + " border-emerald-300 bg-emerald-500 text-white"
	case "present":
		return base + " border-amber-300 bg-amber-500 text-white"
	case "absent":
		return base + " border-slate-500 bg-slate-700 text-white"
	case "tbd":
		return base + " border-white/25 bg-slate-900/80 text-white"
	default:
		return base + " border-white/15 bg-slate-950/60 text-slate-400"
	}
}

func tileAttrs(tile TileView) templ.Attributes {
	attrs := templ.Attributes{}
	if tile.State == "tbd" {
		attrs["data-text"] = fmt.Sprintf("($guess[%d] || '').toUpperCase()", tile.Index)
	}
	return attrs
}

func tileAriaLabel(tile TileView) string {
	if tile.Letter == "" {
		return fmt.Sprintf("Empty tile %d", tile.Index+1)
	}
	state := strings.TrimSpace(tile.State)
	if state == "" {
		return tile.Letter
	}
	return fmt.Sprintf("%s %s", tile.Letter, state)
}

func keyClass(state string) string {
	base := "rounded-lg px-2 py-2 text-xs font-black sm:px-3"
	switch state {
	case "correct":
		return base + " bg-emerald-500 text-white"
	case "present":
		return base + " bg-amber-500 text-white"
	case "absent":
		return base + " bg-slate-700 text-slate-300"
	default:
		return base + " bg-white/10 text-slate-200"
	}
}

func guessCount(used, max int) string {
	return fmt.Sprintf("%d/%d", used, max)
}
