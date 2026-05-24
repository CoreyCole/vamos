package artifacts

import (
	"os"
	"path/filepath"

	"github.com/playwright-community/playwright-go"
)

func CaptureScreenshot(page playwright.Page, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(path),
		FullPage: playwright.Bool(true),
	})
	return err
}

func CaptureHTML(page playwright.Page, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	html, err := page.Content()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(html), 0o644)
}
