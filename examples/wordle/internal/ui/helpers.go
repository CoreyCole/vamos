package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a-h/templ"
)

const (
	stateCorrect = "correct"
	statePresent = "present"
	stateAbsent  = "absent"
	stateTBD     = "tbd"

	animationShake = "shake"
	animationWin   = "win"
	animationKey   = "key"
	keyEnter       = "enter"
	keyBackspace   = "backspace"
)

func gameSignals(data GameView) string {
	signals := map[string]string{"guess": data.CurrentGuess}
	encoded, err := json.Marshal(signals)
	if err != nil {
		return `{"guess":""}`
	}
	return string(encoded)
}

func rowClass(row GuessRow) string {
	base := "grid grid-cols-5 gap-2"
	if row.Current {
		return base + " rounded-2xl"
	}
	return base
}

func rowAttrs(row GuessRow) templ.Attributes {
	attrs := templ.Attributes{}
	if row.Current {
		attrs["data-wordle-row"] = "current"
	}
	switch row.Animation {
	case animationShake:
		attrs["data-wordle-animation"] = animationShake
	case animationWin:
		attrs["data-wordle-animation"] = animationWin
	}
	return attrs
}

func tileClass(state string) string {
	base := "grid size-14 place-items-center rounded-xl border text-2xl font-black uppercase sm:size-16 sm:text-3xl"
	switch state {
	case stateCorrect:
		return base + " border-emerald-300 bg-emerald-500 text-white"
	case statePresent:
		return base + " border-amber-300 bg-amber-500 text-white"
	case stateAbsent:
		return base + " border-slate-500 bg-slate-700 text-white"
	case stateTBD:
		return base + " border-white/25 bg-slate-900/80 text-white"
	default:
		return base + " border-white/15 bg-slate-950/60 text-slate-400"
	}
}

func tileAttrs(tile TileView) templ.Attributes {
	attrs := templ.Attributes{}
	if tile.State == stateTBD {
		attrs["data-text"] = fmt.Sprintf("($guess[%d] || '').toUpperCase()", tile.Index)
	}
	if tile.DelayMS > 0 {
		attrs["style"] = fmt.Sprintf("animation-delay: %dms", tile.DelayMS)
	}
	if tile.Animation != "" {
		attrs["data-wordle-animation"] = tile.Animation
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

func physicalKeyboardHandler(canGuess bool) string {
	if !canGuess {
		return "return"
	}
	return "if (/^[a-zA-Z]$/.test(evt.key) && $guess.length < 5) { $guess = ($guess + evt.key.toLowerCase()).slice(0, 5) } else if (evt.key === 'Backspace') { $guess = $guess.slice(0, -1) } else if (evt.key === 'Enter' && $guess.length === 5) { document.getElementById('guess-form')?.requestSubmit() }"
}

func keyClass(key KeyboardKey) string {
	base := "rounded-lg py-2 text-xs font-black uppercase transition active:scale-95 disabled:opacity-50 "
	if key.Wide {
		base += "px-3 sm:px-5"
	} else {
		base += "min-w-8 px-2 sm:min-w-10 sm:px-3"
	}
	switch key.State {
	case "correct":
		return base + " bg-emerald-500 text-white"
	case "present":
		return base + " bg-amber-500 text-white"
	case "absent":
		return base + " bg-slate-700 text-slate-300"
	default:
		return base + " bg-white/10 text-slate-200 hover:bg-white/15"
	}
}

func keyAttrs(key KeyboardKey) templ.Attributes {
	attrs := templ.Attributes{}
	switch key.Value {
	case keyEnter:
		attrs["data-on:click"] = "if ($guess.length === 5) { document.getElementById('guess-form')?.requestSubmit() }"
	case keyBackspace:
		attrs["data-on:click"] = "$guess = $guess.slice(0, -1)"
	default:
		if len(key.Value) == 1 {
			attrs["data-on:click"] = fmt.Sprintf(
				"$guess = ($guess + '%s').slice(0, 5)",
				strings.ToLower(key.Value),
			)
		}
	}
	if key.DelayMS > 0 {
		attrs["style"] = fmt.Sprintf("animation-delay: %dms", key.DelayMS)
		attrs["data-wordle-animation"] = animationKey
	}
	return attrs
}

func keyAriaLabel(key KeyboardKey) string {
	switch key.Value {
	case keyEnter:
		return "Submit guess"
	case keyBackspace:
		return "Delete letter"
	default:
		return "Letter " + key.Label
	}
}

func guessCount(used, total int) string {
	return fmt.Sprintf("%d/%d", used, total)
}
