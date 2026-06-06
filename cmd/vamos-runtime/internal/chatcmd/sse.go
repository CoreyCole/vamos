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

type StreamEvent struct {
	ID        int64
	Name      string
	ChatEvent chatsession.ChatEvent
	Raw       json.RawMessage
}

func StreamRunNDJSON(ctx context.Context, client APIClient, keyID, secret string, ref ChatRunRef, out io.Writer) error {
	after := ref.EventAfter
	for {
		resp, err := client.Events(ctx, keyID, secret, ref.ChatSessionID, after)
		if err != nil {
			return err
		}
		err = ReadSSEEvents(ctx, resp.Body, func(streamEvent StreamEvent) error {
			if streamEvent.ID > 0 {
				after = streamEvent.ID
			}
			if streamEvent.Name != "chat-session-event" {
				return nil
			}
			event := streamEvent.ChatEvent
			if event.Seq > 0 {
				after = event.Seq
			}
			if err := WriteNDJSON(out, ndjsonFromChatEvent(event, ref)); err != nil {
				return err
			}
			if !isTerminalRunEvent(event, ref.RunID) {
				return nil
			}
			snapshot, snapErr := client.Snapshot(ctx, keyID, secret, ref.ChatSessionID)
			if snapErr != nil {
				return snapErr
			}
			status := "complete"
			errText := ""
			if event.EventType == chatsession.EventRunFailed {
				status = "failed"
				errText = eventError(event.PayloadJSON)
			}
			if err := WriteNDJSON(out, terminalNDJSON(status, ref, snapshot, event.RunID, errText)); err != nil {
				return err
			}
			return errTerminalSeen
		})
		_ = resp.Body.Close()
		if err == nil || err == io.ErrUnexpectedEOF {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			continue
		}
		if errorsIsTerminalSeen(err) {
			return nil
		}
		return err
	}
}

var errTerminalSeen = fmt.Errorf("terminal chat run event seen")

func errorsIsTerminalSeen(err error) bool { return err == errTerminalSeen }

func isTerminalRunEvent(event chatsession.ChatEvent, runID string) bool {
	if event.EventType != chatsession.EventRunCompleted && event.EventType != chatsession.EventRunFailed {
		return false
	}
	return strings.TrimSpace(runID) == "" || strings.TrimSpace(event.RunID) == "" || event.RunID == runID
}

func ReadSSEEvents(ctx context.Context, r io.Reader, emit func(StreamEvent) error) error {
	scanner := bufio.NewScanner(r)
	var eventName string
	var data strings.Builder
	var id int64
	flush := func() error {
		if strings.TrimSpace(eventName) == "" && strings.TrimSpace(data.String()) == "" {
			return nil
		}
		streamEvent := StreamEvent{Name: strings.TrimSpace(eventName), ID: id, Raw: json.RawMessage(strings.TrimSpace(data.String()))}
		if streamEvent.Name == "chat-session-event" && len(streamEvent.Raw) > 0 {
			if err := json.Unmarshal(streamEvent.Raw, &streamEvent.ChatEvent); err != nil {
				return err
			}
			if streamEvent.ID == 0 {
				streamEvent.ID = streamEvent.ChatEvent.Seq
			}
		}
		return emit(streamEvent)
	}
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			eventName = ""
			data.Reset()
			id = 0
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "id:") {
			_, _ = fmt.Sscan(strings.TrimSpace(strings.TrimPrefix(line, "id:")), &id)
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
		return err
	}
	if strings.TrimSpace(eventName) != "" || strings.TrimSpace(data.String()) != "" {
		if err := flush(); err != nil {
			return err
		}
	}
	return io.ErrUnexpectedEOF
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
	var completion ChatCompletion
	err := ReadSSEEvents(ctx, r, func(streamEvent StreamEvent) error {
		if streamEvent.Name != "chat-session-event" {
			return nil
		}
		switch streamEvent.ChatEvent.EventType {
		case chatsession.EventRunCompleted:
			completion = ChatCompletion{RunID: streamEvent.ChatEvent.RunID}
			return errTerminalSeen
		case chatsession.EventRunFailed:
			completion = ChatCompletion{RunID: streamEvent.ChatEvent.RunID, Failed: true, Error: eventError(streamEvent.ChatEvent.PayloadJSON)}
			return errTerminalSeen
		default:
			return nil
		}
	})
	if errorsIsTerminalSeen(err) {
		return completion, nil
	}
	return ChatCompletion{}, err
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
