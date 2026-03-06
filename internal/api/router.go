package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/qubic/network-guardians/internal/cache"
	"github.com/qubic/network-guardians/internal/config"
	"github.com/qubic/network-guardians/internal/repository/postgres"
	"github.com/qubic/network-guardians/internal/service/epoch"
	"github.com/qubic/network-guardians/internal/service/reference"
)

func NewRouter(
	nodeRepo *postgres.NodeRepository,
	epochRepo *postgres.EpochRepository,
	referenceSvc *reference.Service,
	epochSvc *epoch.Service,
	epochCfg *config.EpochConfig,
	scoringCfg *config.ScoringConfig,
	apiCache *cache.Cache,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(corsMiddleware)

	h := NewHandlers(nodeRepo, epochRepo, referenceSvc, epochSvc, epochCfg, scoringCfg, apiCache)

	// Health endpoint
	r.Get("/health", h.HealthCheck)

	// API routes (public)
	r.Route("/api/v1", func(r chi.Router) {
		// Nodes
		r.Get("/nodes", h.ListNodes)
		r.Get("/nodes/{operator}/{type}", h.GetNode)

		// Leaderboard
		r.Get("/leaderboard", h.GetLeaderboard)
		r.Get("/leaderboard/{epoch}", h.GetHistoricalLeaderboard)

		// Stats
		r.Get("/stats", h.GetStats)
	})

	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
