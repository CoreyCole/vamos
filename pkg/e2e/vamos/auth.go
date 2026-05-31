package vamos

import (
	"context"
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"
)

func AuthenticatedAs(email string) spec.Step {
	return spec.Custom("authenticated as "+email, func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		if err := ctx.Config.App.Authenticate(context.Background(), ctx.Page, ctx.Config, email); err != nil {
			t.Fatal(err)
		}
	})
}
