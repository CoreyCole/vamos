package tests

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
	"github.com/playwright-community/playwright-go"
)

type streamlitBrowserProbe struct {
	mu                      sync.Mutex
	urls, errors, responses []string
}

func (p *streamlitBrowserProbe) observeWebSocket(ws playwright.WebSocket) {
	if strings.Contains(ws.URL(), "_stcore/stream") {
		p.mu.Lock()
		p.urls = append(p.urls, ws.URL())
		p.mu.Unlock()
		ws.OnSocketError(
			func(m string) { p.mu.Lock(); p.errors = append(p.errors, m); p.mu.Unlock() },
		)
	}
}

func (p *streamlitBrowserProbe) observeResponse(r playwright.Response) {
	if strings.Contains(r.URL(), "_stcore/") && r.Status() >= http.StatusBadRequest {
		p.mu.Lock()
		p.responses = append(p.responses, r.URL())
		p.mu.Unlock()
	}
}

func (p *streamlitBrowserProbe) observeRequestFailure(r playwright.Request) {
	if strings.Contains(r.URL(), "_stcore/") {
		p.mu.Lock()
		p.errors = append(p.errors, r.URL())
		p.mu.Unlock()
	}
}

func observeStreamlitBrowserTraffic(p *streamlitBrowserProbe) spec.Step {
	return spec.Custom(
		"observe Streamlit browser traffic",
		func(t testing.TB, c *duiruntime.Context) {
			c.Page.OnWebSocket(p.observeWebSocket)
			c.Page.OnResponse(p.observeResponse)
			c.Page.OnRequestFailed(p.observeRequestFailure)
		},
	)
}

func expectStreamlitNoWebSocketFailures(p *streamlitBrowserProbe) spec.Step {
	return spec.Custom(
		"Streamlit websocket has no failures",
		func(t testing.TB, c *duiruntime.Context) {
			time.Sleep(500 * time.Millisecond)
			p.mu.Lock()
			defer p.mu.Unlock()
			if len(p.urls) == 0 || len(p.errors) > 0 || len(p.responses) > 0 {
				t.Fatalf(
					"streamlit traffic urls=%v errors=%v responses=%v",
					p.urls,
					p.errors,
					p.responses,
				)
			}
		},
	)
}

type streamlitProcessIdentity struct{ PID, RunID, StartedAt, Text string }

func readStreamlitProcessIdentityForFrame(
	c *duiruntime.Context,
	selector string,
) (streamlitProcessIdentity, error) {
	raw, err := c.Page.FrameLocator(selector).
		GetByText("Process PID", playwright.FrameLocatorGetByTextOptions{Exact: playwright.Bool(false)}).
		First().
		Evaluate(`marker => { const text=(marker.ownerDocument.querySelector('main, body').innerText)||''; const pick=r => (text.match(r)||[])[1]||''; return {pid:pick(/Process PID\s+(\d+)/),runID:pick(/"run_id"\s*:\s*"([^"]+)"/),startedAt:pick(/"started_at"\s*:\s*"([^"]+)"/),text} }`, nil)
	if err != nil {
		return streamlitProcessIdentity{}, err
	}
	d, ok := raw.(map[string]any)
	if !ok {
		return streamlitProcessIdentity{Text: fmt.Sprint(raw)}, nil
	}
	return streamlitProcessIdentity{
		PID:       fmt.Sprint(d["pid"]),
		RunID:     fmt.Sprint(d["runID"]),
		StartedAt: fmt.Sprint(d["startedAt"]),
		Text:      fmt.Sprint(d["text"]),
	}, nil
}

func visit(t testing.TB, c *duiruntime.Context, p string) {
	t.Helper()
	if _, err := c.Page.Goto(
		strings.TrimRight(c.Config.BaseURL, "/")+p,
		playwright.PageGotoOptions{WaitUntil: playwright.WaitUntilStateDomcontentloaded},
	); err != nil {
		t.Fatal(err)
	}
}

func selectIframeText(frameSelector, textSelector string) spec.Step {
	return spec.Custom("select iframe text", func(t testing.TB, c *duiruntime.Context) {
		target := c.Page.FrameLocator(frameSelector).Locator(textSelector).First()
		if _, err := target.Evaluate(
			`el => { const r=document.createRange(); r.selectNodeContents(el); const s=window.getSelection(); s.removeAllRanges(); s.addRange(r); el.dispatchEvent(new Event('mouseup',{bubbles:true})) }`,
			nil,
		); err != nil {
			t.Fatal(err)
		}
	})
}
