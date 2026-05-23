package layoutprefs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/layouts/workbench"
)

type Service struct {
	queries db.Querier
}

func NewService(queries db.Querier) *Service {
	return &Service{queries: queries}
}

type Input struct {
	UserEmail string
	Page      workbench.WorkbenchPage
	View      workbench.WorkbenchView
	Config    workbench.WorkbenchConfig
}

func stripForStorage(config workbench.WorkbenchConfig) workbench.WorkbenchConfig {
	defaults := workbench.DefaultWorkbenchConfig(config.Page, config.View, "")
	return workbench.StripDurableInteractionState(config, defaults)
}

func (s *Service) Get(
	ctx context.Context,
	userEmail string,
	page workbench.WorkbenchPage,
	view workbench.WorkbenchView,
) (*workbench.WorkbenchConfig, error) {
	row, err := s.queries.GetLayoutPreference(ctx, db.GetLayoutPreferenceParams{
		UserEmail: userEmail,
		Page:      string(page),
		View:      string(view),
	})
	if err != nil {
		return nil, err
	}
	var cfg workbench.WorkbenchConfig
	if err := json.Unmarshal([]byte(row.ConfigJson), &cfg); err != nil {
		return nil, err
	}
	if err := workbench.ValidateWorkbenchConfig(cfg); err != nil {
		return nil, err
	}
	stripped := stripForStorage(cfg)
	return &stripped, workbench.ValidateWorkbenchConfig(stripped)
}

func (s *Service) GetOrDefault(
	ctx context.Context,
	userEmail string,
	page workbench.WorkbenchPage,
	view workbench.WorkbenchView,
	contextMode string,
) workbench.WorkbenchConfig {
	cfg, err := s.Get(ctx, userEmail, page, view)
	defaults := workbench.DefaultWorkbenchConfig(page, view, contextMode)
	if err != nil || cfg == nil {
		return defaults
	}
	return workbench.MergeWorkbenchConfig(defaults, cfg)
}

func (s *Service) Upsert(
	ctx context.Context,
	input Input,
) (workbench.WorkbenchConfig, error) {
	if err := workbench.ValidateWorkbenchConfig(input.Config); err != nil {
		return workbench.WorkbenchConfig{}, err
	}
	stripped := stripForStorage(input.Config)
	payload, err := json.Marshal(stripped)
	if err != nil {
		return workbench.WorkbenchConfig{}, err
	}
	row, err := s.queries.UpsertLayoutPreference(ctx, db.UpsertLayoutPreferenceParams{
		UserEmail:  input.UserEmail,
		Page:       string(input.Page),
		View:       string(input.View),
		ConfigJson: string(payload),
	})
	if err != nil {
		return workbench.WorkbenchConfig{}, err
	}
	var saved workbench.WorkbenchConfig
	if err := json.Unmarshal([]byte(row.ConfigJson), &saved); err != nil {
		return workbench.WorkbenchConfig{}, err
	}
	if err := workbench.ValidateWorkbenchConfig(saved); err != nil {
		return workbench.WorkbenchConfig{}, err
	}
	stripped = stripForStorage(saved)
	return stripped, workbench.ValidateWorkbenchConfig(stripped)
}

func (s *Service) Reset(
	ctx context.Context,
	userEmail string,
	page workbench.WorkbenchPage,
	view workbench.WorkbenchView,
) error {
	err := s.queries.DeleteLayoutPreference(ctx, db.DeleteLayoutPreferenceParams{
		UserEmail: userEmail,
		Page:      string(page),
		View:      string(view),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
