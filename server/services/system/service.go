package system

import (
	"database/sql"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

// Service provides system health monitoring with persistent metrics history.
type Service struct {
	startedAt time.Time
	queries   *db.Queries
	bootID    string
}

// NewService creates a new system health service.
// It starts background goroutines for periodic snapshot capture and startup cleanup.
func NewService(database *sql.DB) *Service {
	svc := &Service{
		startedAt: time.Now(),
		queries:   db.New(database),
		bootID:    readBootID(),
	}
	go svc.cleanupOnStartup()
	go svc.runSnapshotLoop()
	return svc
}
