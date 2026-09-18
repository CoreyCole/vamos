package roster

import (
	"fmt"
	"strings"
)

func ValidateSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" || slug == "." || slug == ".." || strings.ContainsAny(slug, `/\\`) {
		return fmt.Errorf("%w: invalid slug %q", ErrInvalid, slug)
	}
	if slug == "a2a" || strings.HasPrefix(slug, "_") {
		return fmt.Errorf("%w: reserved slug %q", ErrReserved, slug)
	}
	return nil
}
