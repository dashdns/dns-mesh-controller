/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// APIServer serves the blocklist to eBPF DaemonSets via HTTP.
type APIServer struct {
	BlocklistCache *BlocklistCache
	Server         *http.Server
	Client         client.Client
}

// BlocklistResponse represents the API response containing the IP-to-domains mapping.
type BlocklistResponse struct {
	Blocklist []BlocklistEntry `json:"blocklist"`
}

// NewAPIServer creates a new API server instance.
func NewAPIServer(blocklistCache *BlocklistCache, addr string, client client.Client) *APIServer {
	apiServer := &APIServer{
		BlocklistCache: blocklistCache,
		Client:         client,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/policies", apiServer.handleGetBlocklist)
	mux.HandleFunc("/healthz", apiServer.handleHealthz)

	apiServer.Server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return apiServer
}

// Start starts the API server.
func (s *APIServer) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx).WithName("api-server")
	log.Info("Starting API server", "addr", s.Server.Addr)

	// Start server in goroutine
	go func() {
		if err := s.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "API server failed")
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Info("Shutting down API server")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Server.Shutdown(shutdownCtx); err != nil {
		log.Error(err, "API server shutdown failed")
		return err
	}

	return nil
}

// handleGetBlocklist handles GET /api/policies
// Returns the full blocklist of IP-to-domains mappings.
func (s *APIServer) handleGetBlocklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the blocklist from cache
	blocklist := s.BlocklistCache.GetBlocklist()

	// Create response
	response := BlocklistResponse{
		Blocklist: blocklist,
	}

	// Return response as JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// handleHealthz handles GET /healthz
func (s *APIServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "ok",
		"blocklist_entries": s.BlocklistCache.Size(),
	})
}
