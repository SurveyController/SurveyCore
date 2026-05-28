package providers

import (
	"fmt"
	"sync"

	"github.com/SurveyController/SurveyController-Go/internal/models"
	"github.com/SurveyController/SurveyController-Go/internal/providers/credamo"
	"github.com/SurveyController/SurveyController-Go/internal/providers/tencent"
	"github.com/SurveyController/SurveyController-Go/internal/providers/wjx"
)

// Registry manages the mapping of provider names to adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]models.ProviderAdapter
}

// NewRegistry creates a new provider registry with all built-in providers.
func NewRegistry() *Registry {
	r := &Registry{
		adapters: make(map[string]models.ProviderAdapter),
	}
	// Register built-in providers
	r.Register(wjx.NewProvider())
	r.Register(tencent.NewProvider())
	r.Register(credamo.NewProvider())
	return r
}

// Register adds a provider adapter to the registry.
func (r *Registry) Register(adapter models.ProviderAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[adapter.ProviderName()] = adapter
}

// Get retrieves a provider adapter by name.
func (r *Registry) Get(providerName string) (models.ProviderAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[providerName]
	if !ok {
		return nil, fmt.Errorf("unsupported survey provider: %s", providerName)
	}
	return adapter, nil
}

// GetByURL detects the provider from a URL and returns the adapter.
func (r *Registry) GetByURL(urlValue string) (models.ProviderAdapter, error) {
	providerName := DetectSurveyProvider(urlValue, ProviderWJX)
	return r.Get(providerName)
}

// DefaultRegistry is the package-level default registry.
var defaultRegistry = NewRegistry()

// Default returns the default provider registry.
func Default() *Registry {
	return defaultRegistry
}
