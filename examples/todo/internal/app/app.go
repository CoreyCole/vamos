package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"example.com/vamos-datastar-starter/internal/store"
	"example.com/vamos-datastar-starter/internal/store/dbgen"
	"example.com/vamos-datastar-starter/internal/ui"
)

type Config struct {
	FilesRoot string
}

type Service struct {
	filesRoot string
	db        *sql.DB
	queries   *dbgen.Queries
	notifier  *notifier
}

func New(cfg Config) (*Service, error) {
	root := strings.TrimSpace(cfg.FilesRoot)
	if root == "" {
		root = "./files"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create files root: %w", err)
	}

	database, err := store.Open(filepath.Join(root, "app.db"))
	if err != nil {
		return nil, err
	}
	service := &Service{
		filesRoot: root,
		db:        database,
		queries:   dbgen.New(database),
		notifier:  newNotifier(),
	}
	if err := service.ensureStarterData(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	return service, nil
}

func (s *Service) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Service) pageData(ctx context.Context, message string) (ui.PageData, error) {
	rows, err := s.queries.ListItems(ctx)
	if err != nil {
		return ui.PageData{}, err
	}
	items := make([]ui.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, ui.Item{
			ID:        row.ID,
			Title:     row.Title,
			Completed: row.Completed != 0,
		})
	}
	if message == "" {
		message = "Backend state rendered through SSE stream."
	}
	return ui.PageData{Items: items, Message: message}, nil
}

func (s *Service) ensureStarterData(ctx context.Context) error {
	count, err := s.queries.CountItems(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = s.queries.CreateItem(ctx, "Run just build")
	if err != nil {
		return err
	}
	_, err = s.queries.CreateItem(ctx, "Edit a templ component")
	return err
}

type notifier struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func newNotifier() *notifier {
	return &notifier{subs: map[chan struct{}]struct{}{}}
}

func (n *notifier) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	n.subs[ch] = struct{}{}
	n.mu.Unlock()
	return ch
}

func (n *notifier) unsubscribe(ch chan struct{}) {
	n.mu.Lock()
	delete(n.subs, ch)
	close(ch)
	n.mu.Unlock()
}

func (n *notifier) notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func parseID(raw string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &id)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
