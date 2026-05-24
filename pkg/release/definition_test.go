package release

import (
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

func serviceWorkflow(includePush bool) runtime.Definition {
	b := runtime.New[struct{}]("release.test").
		Start("preflight").
		Service("preflight", runtime.ServiceSpec{Type: "release.preflight"}).
		Done("done").
		Edge("preflight", "done")
	if includePush {
		b = runtime.New[struct{}]("release.test").
			Start("preflight").
			Service("preflight", runtime.ServiceSpec{Type: "release.preflight"}).
			Service("push", runtime.ServiceSpec{Type: "release.push"}).
			Done("done").
			Edge("preflight", "push").
			Edge("push", "done")
	}
	return b.MustBuild()
}

func workflowRegistry(t *testing.T, def runtime.Definition) *runtime.Registry {
	t.Helper()
	reg := runtime.NewRegistry()
	if err := reg.Register(def); err != nil {
		t.Fatalf("Register workflow: %v", err)
	}
	return reg
}

func TestDefinitionValidation(t *testing.T) {
	reg := workflowRegistry(t, serviceWorkflow(true))
	tests := []struct {
		name    string
		build   func() (Definition, error)
		wantErr string
	}{
		{name: "valid arbitrary slugs", build: func() (Definition, error) {
			return NewDefinition("default").Lane("stage", CheckoutSlug("worktree-stage")).Lane("main", CheckoutSlug("prod-main"), Protected()).Flow("release", "release.test", FromLane("stage"), ToLane("main"), PushAfterVerifyPolicy()).Build(reg)
		}},
		{name: "duplicate lane", build: func() (Definition, error) { return NewDefinition("default").Lane("stage").Lane("stage").Build(reg) }, wantErr: "duplicate lane"},
		{name: "duplicate flow", build: func() (Definition, error) {
			return NewDefinition("default").Lane("stage").Lane("main").Flow("promote", "release.test", FromFeature(), ToLane("stage")).Flow("promote", "release.test", FromFeature(), ToLane("stage")).Build(reg)
		}, wantErr: "duplicate flow"},
		{name: "missing workflow", build: func() (Definition, error) {
			return NewDefinition("default").Lane("stage").Flow("promote", "missing", FromFeature(), ToLane("stage")).Build(reg)
		}, wantErr: "is not registered"},
		{name: "missing target", build: func() (Definition, error) {
			return NewDefinition("default").Flow("promote", "release.test", FromFeature(), ToLane("stage")).Build(reg)
		}, wantErr: "target lane"},
		{name: "protected source", build: func() (Definition, error) {
			return NewDefinition("default").Lane("main", Protected()).Lane("stage").Flow("bad", "release.test", FromLane("main"), ToLane("stage")).Build(reg)
		}, wantErr: "protected source"},
		{name: "push requires push node", build: func() (Definition, error) {
			noPushReg := workflowRegistry(t, serviceWorkflow(false))
			return NewDefinition("default").Lane("stage").Lane("main").Flow("release", "release.test", FromLane("stage"), ToLane("main"), PushAfterVerifyPolicy()).Build(noPushReg)
		}, wantErr: "release.push"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.build()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Build() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Build() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	reg := workflowRegistry(t, serviceWorkflow(false))
	def := NewDefinition("default").Version("v1").Lane("stage").Flow("promote", "release.test", FromFeature(), ToLane("stage")).MustBuild(reg)
	releases := NewRegistry(reg)
	if err := releases.Register(def); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got, ok := releases.Definition("default", "v1"); !ok || got.ID != "default" {
		t.Fatalf("Definition() = (%v,%v), want default", got.ID, ok)
	}
	if err := releases.Register(def); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate Register err = %v", err)
	}
}
