package agentchat

import (
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

func ApplyChatQuote(prompt, quoteText, quotePath string) string {
	prompt = strings.TrimSpace(prompt)
	quoteText = strings.TrimSpace(quoteText)
	if quoteText == "" {
		return prompt
	}
	name := filepath.Base(strings.TrimSpace(quotePath))
	quoted := "> " + strings.ReplaceAll(quoteText, "\n", "\n> ")
	block := quoted
	if name != "" && name != "." {
		block = name + "\n" + quoted
	}
	if prompt == "" {
		return block
	}
	return block + "\n\n" + prompt
}

func chatPromptFromForm(c echo.Context) string {
	return ApplyChatQuote(
		strings.TrimSpace(c.FormValue("prompt")),
		c.FormValue("chat_quote_text"),
		c.FormValue("chat_quote_path"),
	)
}
