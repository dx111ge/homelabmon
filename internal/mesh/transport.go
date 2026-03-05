package mesh

import (
	"context"
	"net/http"
	"time"

	"github.com/dx111ge/homelabmon/internal/agent"
	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/store"
	"github.com/rs/zerolog/log"
)

// Transport handles HTTP communication between mesh nodes.
type Transport struct {
	identity  *models.NodeIdentity
	store     *store.Store
	collector *agent.Collector
	server    *http.Server
	mux       *http.ServeMux
}

func NewTransport(identity *models.NodeIdentity, s *store.Store, collector *agent.Collector) *Transport {
	t := &Transport{
		identity:  identity,
		store:     s,
		collector: collector,
		mux:       http.NewServeMux(),
	}
	t.setupRoutes()
	return t
}

// Mux returns the HTTP mux so the UI server can mount additional routes.
func (t *Transport) Mux() *http.ServeMux {
	return t.mux
}

func (t *Transport) Start(bindAddr string) error {
	t.server = &http.Server{
		Addr:         bindAddr,
		Handler:      t.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("bind", bindAddr).Msg("transport listening")
		if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("transport failed")
		}
	}()
	return nil
}

func (t *Transport) Stop(ctx context.Context) error {
	if t.server != nil {
		return t.server.Shutdown(ctx)
	}
	return nil
}
