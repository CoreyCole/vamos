package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"example.com/vamos-datastar-starter/internal/app"
)

func main() {
	service, err := app.New(app.Config{
		FilesRoot: filesRoot(),
	})
	if err != nil {
		log.Fatalf("initialize app: %v", err)
	}
	defer service.Close()

	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if addr == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "8080"
		}
		addr = "0.0.0.0:" + port
	}

	e := service.Routes()
	log.Printf("datastar starter listening on http://%s", addr)
	if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

func filesRoot() string {
	if root := strings.TrimSpace(os.Getenv("VAMOS_APP_FILES_ROOT")); root != "" {
		return root
	}
	return "./files"
}
