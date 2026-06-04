package chatcmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
)

type ChatCompletion struct {
	RunID    string
	Failed   bool
	Error    string
	Response string
}

type ChatSessionWatcher struct {
	HTTPClient *http.Client
}

type snapshotResponse struct {
	Projection chatsession.ChatProjection `json:"projection"`
	LastSeq    int64                      `json:"last_seq"`
}

func (w ChatSessionWatcher) WatchUntilComplete(ctx context.Context, baseURL, sessionID string, afterSeq int64) (ChatCompletion, error) {
	client := w.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	eventsURL, err := url.Parse(strings.TrimRight(baseURL, "/") + "/agent-chat/chat-sessions/" + url.PathEscape(sessionID) + "/events")
	if err != nil {
		return ChatCompletion{}, err
	}
	q := eventsURL.Query()
	q.Set("after", fmt.Sprint(afterSeq))
	eventsURL.RawQuery = q.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, eventsURL.String(), nil)
	if err != nil {
		return ChatCompletion{}, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return ChatCompletion{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ChatCompletion{}, fmt.Errorf("chat session SSE failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	completion, err := readCompletionEvent(ctx, resp.Body)
	if err != nil {
		return ChatCompletion{}, err
	}
	snapshot, err := w.Snapshot(ctx, baseURL, sessionID)
	if err != nil {
		return ChatCompletion{}, err
	}
	completion.Response = finalAssistantResponse(snapshot.Projection, completion.RunID)
	return completion, nil
}

func (w ChatSessionWatcher) Snapshot(ctx context.Context, baseURL, sessionID string) (snapshotResponse, error) {
	client := w.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	snapshotURL := strings.TrimRight(baseURL, "/") + "/agent-chat/chat-sessions/" + url.PathEscape(sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, snapshotURL, nil)
	if err != nil {
		return snapshotResponse{}, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return snapshotResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return snapshotResponse{}, fmt.Errorf("chat session snapshot failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out snapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return snapshotResponse{}, err
	}
	return out, nil
}

func readCompletionEvent(ctx context.Context, r io.Reader) (ChatCompletion, error) {
	scanner := bufio.NewScanner(r)
	var eventName string
	var data strings.Builder
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ChatCompletion{}, ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			completion, done, err := completionFromSSE(eventName, data.String())
			if err != nil {
				return ChatCompletion{}, err
			}
			if done {
				return completion, nil
			}
			eventName = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatCompletion{}, err
	}
	return ChatCompletion{}, io.ErrUnexpectedEOF
}

func completionFromSSE(eventName, data string) (ChatCompletion, bool, error) {
	if strings.TrimSpace(eventName) != "chat-session-event" || strings.TrimSpace(data) == "" {
		return ChatCompletion{}, false, nil
	}
	var event chatsession.ChatEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return ChatCompletion{}, false, err
	}
	switch event.EventType {
	case chatsession.EventRunCompleted:
		return ChatCompletion{RunID: event.RunID}, true, nil
	case chatsession.EventRunFailed:
		return ChatCompletion{RunID: event.RunID, Failed: true, Error: eventError(event.PayloadJSON)}, true, nil
	default:
		return ChatCompletion{}, false, nil
	}
}

func eventError(payload json.RawMessage) string {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return strings.TrimSpace(string(payload))
	}
	for _, key := range []string{"error", "message", "summary"} {
		if value, ok := body[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return strings.TrimSpace(string(payload))
}

func finalAssistantResponse(proj chatsession.ChatProjection, runID string) string {
	for i := len(proj.Messages) - 1; i >= 0; i-- {
		message := proj.Messages[i]
		if message.Role != "assistant" || strings.TrimSpace(message.Content) == "" {
			continue
		}
		if strings.TrimSpace(runID) == "" || strings.TrimSpace(message.RunID) == "" || message.RunID == runID {
			return strings.TrimSpace(message.Content)
		}
	}
	return ""
}

func httpClientWithCookies(cookies []*http.Cookie) *http.Client {
	return &http.Client{Transport: cookieTransport{base: http.DefaultTransport, cookies: cookies}}
}

type cookieTransport struct {
	base    http.RoundTripper
	cookies []*http.Cookie
}

func (t cookieTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for _, cookie := range t.cookies {
		req.AddCookie(cookie)
	}
	return t.base.RoundTrip(req)
}
