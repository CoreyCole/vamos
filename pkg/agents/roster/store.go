package roster

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrNotFound = errors.New("roster: bot not found")
	ErrArchived = errors.New("roster: bot archived")
	ErrReserved = errors.New("roster: reserved slug")
	ErrConflict = errors.New("roster: slug conflict")
	ErrInvalid  = errors.New("roster: invalid document")
)

type Bot struct {
	Slug        string `yaml:"slug"`
	Name        string `yaml:"name"`
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	Archived    bool   `yaml:"archived"`
}

type Document struct {
	Bots []Bot `yaml:"bots"`
}

type Store struct {
	Path string
}

func (s *Store) List() ([]Bot, error) {
	doc, err := s.load()
	if err != nil {
		return nil, err
	}
	active := make([]Bot, 0, len(doc.Bots))
	for _, bot := range doc.Bots {
		if bot.Archived {
			continue
		}
		active = append(active, bot)
	}
	sort.Slice(active, func(i, j int) bool {
		ni := strings.ToLower(active[i].Name)
		nj := strings.ToLower(active[j].Name)
		if ni != nj {
			return ni < nj
		}
		return active[i].Slug < active[j].Slug
	})
	return active, nil
}

func (s *Store) Get(slug string) (Bot, error) {
	doc, err := s.load()
	if err != nil {
		return Bot{}, err
	}
	var found *Bot
	for i := range doc.Bots {
		if doc.Bots[i].Slug == slug {
			found = &doc.Bots[i]
			break
		}
	}
	if found == nil {
		return Bot{}, ErrNotFound
	}
	if found.Archived {
		return Bot{}, ErrArchived
	}
	return *found, nil
}

func (s *Store) load() (Document, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Document{}, nil
		}
		return Document{}, err
	}
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	seen := make(map[string]struct{}, len(doc.Bots))
	for _, bot := range doc.Bots {
		if err := ValidateSlug(bot.Slug); err != nil {
			return Document{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if _, ok := seen[bot.Slug]; ok {
			return Document{}, fmt.Errorf("%w: duplicate slug %q", ErrInvalid, bot.Slug)
		}
		seen[bot.Slug] = struct{}{}
	}
	return doc, nil
}
