package provider

import (
	"fmt"

	"github.com/pendig/rute-bayar/internal/domain"
)

type Registry struct {
	adapters map[domain.ProviderCode]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{adapters: make(map[domain.ProviderCode]Adapter, len(adapters))}
	for _, adapter := range adapters {
		registry.adapters[adapter.Code()] = adapter
	}
	return registry
}

func (r *Registry) Get(code domain.ProviderCode) (Adapter, error) {
	adapter, ok := r.adapters[code]
	if !ok {
		return nil, fmt.Errorf("provider adapter %q is not registered", code)
	}
	return adapter, nil
}

func (r *Registry) List() []Adapter {
	adapters := make([]Adapter, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		adapters = append(adapters, adapter)
	}
	return adapters
}

