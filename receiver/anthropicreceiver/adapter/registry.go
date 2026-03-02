package adapter

import "net/http"

type Registry struct {
	adapters []ProviderAdapter
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) Register(adapter ProviderAdapter) {
	r.adapters = append(r.adapters, adapter)
}

func (r *Registry) DetectProvider(req *http.Request, body []byte) ProviderAdapter {
	for _, adapter := range r.adapters {
		if adapter.Detect(req, body) {
			return adapter
		}
	}
	return nil
}

func (r *Registry) GetProvider(name string) ProviderAdapter {
	for _, adapter := range r.adapters {
		if adapter.Name() == name {
			return adapter
		}
	}
	return nil
}

func (r *Registry) Providers() []ProviderAdapter {
	return r.adapters
}
