package plugin

import (
	"context"
	"sync"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/rs/zerolog/log"
)

// Registry manages plugin lifecycle: register, detect, start, stop.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	states  map[string]models.PluginState
	cancels map[string]context.CancelFunc
}

func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
		states:  make(map[string]models.PluginState),
		cancels: make(map[string]context.CancelFunc),
	}
}

// Register adds a plugin to the registry (does not start it).
func (r *Registry) Register(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[p.Name()] = p
	r.states[p.Name()] = models.PluginRegistered
	log.Debug().Str("plugin", p.Name()).Str("type", string(p.Type())).Msg("plugin registered")
}

// DetectAndStart calls Detect() on each registered plugin and starts those that return true.
func (r *Registry) DetectAndStart(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, p := range r.plugins {
		if r.states[name] == models.PluginRunning {
			continue
		}

		if !p.Detect() {
			r.states[name] = models.PluginSkipped
			log.Debug().Str("plugin", name).Msg("not detected, skipped")
			continue
		}

		if err := p.Start(ctx); err != nil {
			r.states[name] = models.PluginFailed
			log.Error().Err(err).Str("plugin", name).Msg("failed to start")
			continue
		}

		r.states[name] = models.PluginRunning
		log.Info().Str("plugin", name).Str("type", string(p.Type())).Msg("plugin started")
	}
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.plugins[name]
}

// ListByType returns all plugins of a given type.
func (r *Registry) ListByType(t models.PluginType) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Plugin
	for _, p := range r.plugins {
		if p.Type() == t {
			result = append(result, p)
		}
	}
	return result
}

// RunningObservers returns all running Observer plugins.
func (r *Registry) RunningObservers() []Observer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Observer
	for name, p := range r.plugins {
		if r.states[name] == models.PluginRunning {
			if obs, ok := p.(Observer); ok {
				result = append(result, obs)
			}
		}
	}
	return result
}

// Info returns status info for all plugins.
func (r *Registry) Info() []models.PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []models.PluginInfo
	for name, p := range r.plugins {
		now := time.Now()
		result = append(result, models.PluginInfo{
			Name:   name,
			Type:   p.Type(),
			Status: r.states[name],
			LastRun: &now,
		})
	}
	return result
}

// StopAll stops all running plugins.
func (r *Registry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, p := range r.plugins {
		if r.states[name] == models.PluginRunning {
			if err := p.Stop(); err != nil {
				log.Error().Err(err).Str("plugin", name).Msg("failed to stop")
			}
			r.states[name] = models.PluginStopped
			log.Info().Str("plugin", name).Msg("plugin stopped")
		}
	}
}
