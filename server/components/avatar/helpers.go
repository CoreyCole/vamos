package avatar

import (
	"strings"
)

// GetEmailBasename extracts the username prefix from an email address
// e.g., "corey@chestnutfi.com" -> "corey"
func GetEmailBasename(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 0 {
		return parts[0]
	}
	return email
}

// GetInitials extracts up to 2 initials from a username
// e.g., "corey" -> "CO", "john.doe" -> "JD"
func GetInitials(username string) string {
	// Replace common separators with spaces
	username = strings.ReplaceAll(username, ".", " ")
	username = strings.ReplaceAll(username, "_", " ")
	username = strings.ReplaceAll(username, "-", " ")

	// Split into parts
	parts := strings.Fields(username)

	if len(parts) == 0 {
		return "??"
	}

	if len(parts) == 1 {
		// Single word: take first 2 characters
		word := strings.ToUpper(parts[0])
		if len(word) >= 2 {
			return word[:2]
		}
		return word
	}

	// Multiple words: take first letter of first two words
	initials := ""
	for i := 0; i < 2 && i < len(parts); i++ {
		if len(parts[i]) > 0 {
			initials += strings.ToUpper(string(parts[i][0]))
		}
	}

	return initials
}

// GetAvatarColor generates a deterministic color from an email address
// Returns a hex color string like "#3b82f6"
func GetAvatarColor(email string) string {
	// Consistent color palette from shadcn/ui
	colors := []string{
		"#3b82f6", // blue-500
		"#10b981", // green-500
		"#f59e0b", // amber-500
		"#ef4444", // red-500
		"#8b5cf6", // violet-500
		"#ec4899", // pink-500
		"#06b6d4", // cyan-500
		"#14b8a6", // teal-500
	}

	// Simple hash function for consistent color per email
	hash := 0
	for _, char := range email {
		hash = hash*31 + int(char)
	}

	// Ensure positive index
	if hash < 0 {
		hash = -hash
	}

	return colors[hash%len(colors)]
}
