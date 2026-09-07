package workbench

import "github.com/a-h/templ"

// threadsOpenSSRHide is SSR-only inline hide when the left rail starts open.
// Datastar data-show takes over after hydrate (no Tailwind hidden class fight).
func threadsOpenSSRHide(threadsOpen bool) templ.SafeCSS {
	if threadsOpen {
		return templ.SafeCSS("display: none;")
	}
	return templ.SafeCSS("")
}
