package release

import (
	"fmt"
	"sort"
	"sync"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
)

type WorkflowRegistry interface {
	GetVersion(id runtime.WorkflowID, version string) (runtime.Definition, bool)
}

type Registry struct {
	mu        sync.RWMutex
	workflows WorkflowRegistry
	defs      map[DefinitionID]map[string]Definition
}

func NewRegistry(workflows WorkflowRegistry) *Registry {
	return &Registry{workflows: workflows, defs: map[DefinitionID]map[string]Definition{}}
}

func (r *Registry) Register(def Definition) error {
	if err := ValidateDefinition(def, r.workflows); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.defs[def.ID] == nil {
		r.defs[def.ID] = map[string]Definition{}
	}
	if _, exists := r.defs[def.ID][def.Version]; exists {
		return fmt.Errorf("release definition %q version %q already registered", def.ID, def.Version)
	}
	r.defs[def.ID][def.Version] = def
	return nil
}

func (r *Registry) Definition(id DefinitionID, version string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.defs[id]
	if len(versions) == 0 {
		return Definition{}, false
	}
	def, ok := versions[version]
	return def, ok
}

func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]Definition, 0)
	for _, versions := range r.defs {
		for _, def := range versions {
			defs = append(defs, def)
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].ID == defs[j].ID {
			return defs[i].Version < defs[j].Version
		}
		return defs[i].ID < defs[j].ID
	})
	return defs
}
