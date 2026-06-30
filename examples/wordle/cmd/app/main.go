package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"example.com/vamos-wordle/internal/app"
)

func main() {
	service, err := app.New(app.Config{
		FilesRoot: filesRoot(),
		WordFile:  wordFile(),
	})
	if err != nil {
		log.Fatalf("initialize app: %v", err)
	}

	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if addr == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "8080"
		}
		addr = "0.0.0.0:" + port
	}

	e := service.Routes()
	log.Println("daily wordle starting")
	if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_ = service.Close()
		log.Fatalf("serve: %v", err)
	}
	if err := service.Close(); err != nil {
		log.Fatalf("close service: %v", err)
	}
}

func filesRoot() string {
	if root := strings.TrimSpace(os.Getenv("VAMOS_APP_FILES_ROOT")); root != "" {
		return root
	}
	return "./files"
}

func wordFile() string {
	if path := strings.TrimSpace(os.Getenv("WORDLE_WORD_FILE")); path != "" {
		return path
	}
	return "internal/words/words.txt"
}
