package markdown

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestMain(m *testing.M) {
	SetEmptyScopeComposer(func(action, attachedDoc, modeLabel string) templ.Component {
		return testEmptyScopeComposer(action, attachedDoc, modeLabel)
	})
	os.Exit(m.Run())
}

func testEmptyScopeComposer(action, attachedDoc, modeLabel string) templ.Component {
	base := filepath.Base(strings.Trim(attachedDoc, "/"))
	path := strings.TrimSpace(attachedDoc)
	if path != "" && !strings.HasPrefix(path, "thoughts/") {
		path = "thoughts/" + strings.Trim(path, "/")
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		var b strings.Builder
		b.WriteString(`<form id="agent-chat-composer" data-on:submit__prevent="`)
		b.WriteString(action)
		b.WriteString(`">`)
		if path != "" && base != "." && base != "/" {
			b.WriteString(`<input type="hidden" name="artifact" value="`)
			b.WriteString(attachedDoc)
			b.WriteString(`"/><input type="hidden" name="attached_paths[]" value="`)
			b.WriteString(path)
			b.WriteString(`"/><span>`)
			b.WriteString(base)
			b.WriteString(`</span>`)
		}
		b.WriteString(`<dt>Mode</dt><dd>`)
		b.WriteString(modeLabel)
		b.WriteString(`</dd>`)
		b.WriteString(
			`<textarea id="agent-chat-composer-input" name="prompt"></textarea>`,
		)
		b.WriteString(
			`<button type="submit" aria-label="Start thread">Send</button></form>`,
		)
		_, err := io.WriteString(w, b.String())
		return err
	})
}
