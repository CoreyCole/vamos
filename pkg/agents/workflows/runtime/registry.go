package runtime

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu   sync.RWMutex
	defs map[WorkflowID]Definition
}

func NewRegistry() *Registry {
	return &Registry{defs: map[WorkflowID]Definition{}}
}

func (r *Registry) Register(def Definition) error {
	if err := ValidateDefinition(def); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.defs[def.ID]; exists {
		return fmt.Errorf("workflow %q already registered", def.ID)
	}
	r.defs[def.ID] = def
	return nil
}

func (r *Registry) Get(id WorkflowID) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.defs[id]
	return def, ok
}
