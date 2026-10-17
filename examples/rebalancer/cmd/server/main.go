package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"example.com/vamos-rebalancer/internal/db"
	"example.com/vamos-rebalancer/internal/web"
)

func main() {
	filesRoot := filesRoot()
	if err := os.MkdirAll(filesRoot, 0o755); err != nil {
		log.Fatalf("create files root: %v", err)
	}

	dbPath := filepath.Join(filesRoot, "rebalancer.db")
	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	schemaPath := filepath.Join("schema.sql")
	if err := db.Initialize(database, schemaPath); err != nil {
		log.Fatalf("initialize database: %v", err)
	}

	app, err := web.New(web.Config{
		DB:        database,
		FilesRoot: filesRoot,
	})
	if err != nil {
		log.Fatalf("initialize app: %v", err)
	}
	defer app.Close()

	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if addr == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "8080"
		}
		addr = "0.0.0.0:" + port
	}

	e := app.Routes()
	log.Printf("portfolio rebalancer starting on %s", addr)
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
