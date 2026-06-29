# Self-modifying Wordle example

This example demonstrates a mobile-friendly Go + Datastar applet whose game state and rules live on the server.

The applet is a long-running Go HTTP server. The browser renders server-sent HTML, submits guesses through short POST requests, and keeps only tiny UI interaction state in Datastar.

## Try the starter applet

From this checkout:

```bash
cd examples/wordle/files/apps/current
go test ./...
PORT=8080 VAMOS_APP_FILES_ROOT="$OLDPWD/examples/wordle/files" go run .
```

Open <http://127.0.0.1:8080/>.

## Gameplay

Wordle gives you six attempts to guess a hidden five-letter word.

- **Green**: the letter is in the word and in the correct spot.
- **Yellow**: the letter is in the word, but in the wrong spot.
- **Gray**: the letter is not in the word at all.

The backend validates guesses against `valid_words.txt`, seeded with NYT Wordle Helper-compatible five-letter words. The browser does not know the answer and does not score guesses.

## Boundary

q-manager owns chat, technical planning, safety checks, build, health check, promotion, recovery, and friendly explanations. The applet owns Wordle rules, server state, dictionary files, and presentation.
