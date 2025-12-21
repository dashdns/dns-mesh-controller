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
	"fmt"
	"net/http"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

// APIServer serves DNS policies to clients via HTTP.
type APIServer struct {
	Index  *PolicyIndex
	Server *http.Server
}

// NewAPIServer creates a new API server instance.
func NewAPIServer(index *PolicyIndex, addr string) *APIServer {
	apiServer := &APIServer{
		Index: index,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/policies", apiServer.handleGetPolicy)
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

// handleGetPolicy handles GET /api/policies?hash=<selectorHash>
func (s *APIServer) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get hash from query parameter
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "Missing 'hash' query parameter", http.StatusBadRequest)
		return
	}

	// Lookup policy by hash
	policy := s.Index.Get(hash)
	if policy == nil {
		http.Error(w, fmt.Sprintf("No policy found for hash: %s", hash), http.StatusNotFound)
		return
	}

	// Return policy as JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(policy); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleHealthz handles GET /healthz
func (s *APIServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"indexed_policies": s.Index.Size(),
	})
}
