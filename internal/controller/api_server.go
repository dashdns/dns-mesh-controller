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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// APIServer serves DNS policies to clients via HTTP.
type APIServer struct {
	Index             *PolicyIndex
	Server            *http.Server
	Client            client.Client
	KubeDnsNamespace  string
	KubeDnsSecretName string
}

// PolicyResponse represents the API response containing policy and TLS data.
type PolicyResponse struct {
	Policy  interface{}    `json:"policy"`
	TLSData *TLSSecretData `json:"tlsData,omitempty"`
}

// TLSSecretData contains the TLS certificate and key data.
type TLSSecretData struct {
	Certificate   []byte `json:"certificate,omitempty"`
	PrivateKey    []byte `json:"privateKey,omitempty"`
	CACertificate []byte `json:"caCertificate,omitempty"`
}

// NewAPIServer creates a new API server instance.
func NewAPIServer(index *PolicyIndex, addr string, client client.Client, kubeDnsNamespace, kubeDnsSecretName string) *APIServer {
	apiServer := &APIServer{
		Index:             index,
		Client:            client,
		KubeDnsNamespace:  kubeDnsNamespace,
		KubeDnsSecretName: kubeDnsSecretName,
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

// fetchTLSSecret fetches the TLS secret from Kubernetes.
func (s *APIServer) fetchTLSSecret(ctx context.Context) (*TLSSecretData, error) {
	secret := &corev1.Secret{}
	namespacedName := types.NamespacedName{
		Namespace: s.KubeDnsNamespace,
		Name:      s.KubeDnsSecretName,
	}

	if err := s.Client.Get(ctx, namespacedName, secret); err != nil {
		return nil, fmt.Errorf("failed to fetch TLS secret: %w", err)
	}

	// Extract certificate, private key, and CA certificate from secret data
	tlsData := &TLSSecretData{
		Certificate:   secret.Data["tls.crt"],
		PrivateKey:    secret.Data["tls.key"],
		CACertificate: secret.Data["ca.crt"],
	}

	return tlsData, nil
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

	// Fetch TLS secret data
	ctx := r.Context()
	tlsData, err := s.fetchTLSSecret(ctx)
	if err != nil {
		// Log error but continue without TLS data
		log := ctrl.LoggerFrom(ctx).WithName("api-server")
		log.Error(err, "Failed to fetch TLS secret, returning policy without TLS data")
		tlsData = nil
	}

	// Create response with policy and TLS data
	response := PolicyResponse{
		Policy:  policy,
		TLSData: tlsData,
	}

	// Return response as JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
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
