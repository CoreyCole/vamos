package vamos

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
)

type WorkspaceFixtureBuilder struct {
	name string
}

func WorkspaceFixture(name string) *WorkspaceFixtureBuilder {
	return &WorkspaceFixtureBuilder{name: name}
}

func (b *WorkspaceFixtureBuilder) SetupStep() spec.Step                                  { return b.Build() }
func (b *WorkspaceFixtureBuilder) WithThoughtsFile(_, _ string) *WorkspaceFixtureBuilder { return b }
func (b *WorkspaceFixtureBuilder) WithChatThread(_ string, _ []Message) *WorkspaceFixtureBuilder {
	return b
}

type Message struct {
	Role    string
	Content string
}

func (b *WorkspaceFixtureBuilder) Build() spec.Step {
	return spec.Custom("workspace fixture "+b.name, func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		ws, err := ReadWorkspaceEnv(ctx.Config.RepoRoot)
		if err != nil {
			t.Fatal(err)
		}
		db, err := OpenWorkspaceDB(t.Context(), ctx.Config)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		workspace := fixtures.WorkspaceIdentity{
			Slug:         ws.Slug,
			CheckoutPath: ws.CheckoutPath,
			DBPath:       ws.DBPath,
			ManagerURL:   ws.ManagerURL,
		}
		thoughtsRoot := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT"))
		if thoughtsRoot == "" {
			thoughtsRoot = filepath.Join(ctx.Config.RepoRoot, "thoughts")
		}
		state, err := fixtures.Load(t.Context(), db, workspace, thoughtsRoot, b.name)
		if err != nil {
			t.Fatal(err)
		}
		storeFixture(ctx, state)
	})
}

func LoadFixture(name string) spec.Step { return WorkspaceFixture(name).Build() }

func storeFixture(ctx *duiruntime.Context, fixture any) {
	state, ok := fixture.(fixtures.State)
	if !ok {
		return
	}
	if ctx.Memory == nil {
		ctx.Memory = map[string]string{}
	}
	ctx.Memory["fixture.name"] = state.Name
	for key, value := range state.Data {
		ctx.Memory["fixture.data."+key] = fmt.Sprint(value)
	}
}

func fixtureFromMemory(ctx *duiruntime.Context) any {
	name := strings.TrimSpace(ctx.Memory["fixture.name"])
	if name == "" {
		return nil
	}
	data := map[string]any{}
	prefix := "fixture.data."
	for key, value := range ctx.Memory {
		if strings.HasPrefix(key, prefix) {
			data[strings.TrimPrefix(key, prefix)] = value
		}
	}
	return fixtures.State{Name: name, Data: data}
}
