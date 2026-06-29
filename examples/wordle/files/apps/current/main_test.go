package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScoreGuessHandlesRepeatedLetters(t *testing.T) {
	got := ScoreGuess("allee", "apple")
	want := []LetterMark{MarkGreen, MarkYellow, MarkGray, MarkGray, MarkGreen}
	for i := range want {
		if got[i].Mark != want[i] {
			t.Fatalf("mark %d = %s, want %s; full result = %#v", i, got[i].Mark, want[i], got)
		}
	}
}

func TestSubmitGuessValidatesDictionaryAndWins(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "valid_words.txt"), []byte("apple\ncrane\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "answers.txt"), []byte("apple\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}

	app.SubmitGuess("xxxxx")
	if len(app.State().Guesses) != 0 {
		t.Fatal("invalid dictionary guess was recorded")
	}
	if !strings.Contains(app.State().Message, "not in the Wordle Helper dictionary") {
		t.Fatalf("invalid message = %q", app.State().Message)
	}

	app.SubmitGuess("apple")
	state := app.State()
	if !state.Over || !state.Won || len(state.Guesses) != 1 {
		t.Fatalf("state after winning guess = %#v", state)
	}
}

func TestRoutesRenderAndAcceptGuess(t *testing.T) {
	root := t.TempDir()
	if err := EnsureStarterFiles(root); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Routes())
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Server Wordle") || !strings.Contains(string(body), "@get('events')") {
		t.Fatalf("GET / status=%d body=%q", resp.StatusCode, string(body))
	}

	postResp, err := http.PostForm(server.URL+"/guess", url.Values{"guess": {"crane"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = postResp.Body.Close()
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /guess final status = %d", postResp.StatusCode)
	}
	if len(app.State().Guesses) != 1 {
		t.Fatalf("guesses recorded = %d, want 1", len(app.State().Guesses))
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/guess", strings.NewReader("guess=crane"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/event-stream")
	ssePostResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = ssePostResp.Body.Close()
	if ssePostResp.StatusCode != http.StatusNoContent {
		t.Fatalf("Datastar POST /guess status = %d, want %d", ssePostResp.StatusCode, http.StatusNoContent)
	}
}

func TestSafeJoinRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	defer func() {
		if recover() == nil {
			t.Fatal("SafeJoin did not panic for escape path")
		}
	}()
	_ = SafeJoin(root, "../answers.txt")
}
