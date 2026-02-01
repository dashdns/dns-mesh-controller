package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emirozbir/sidecar-injector/pkg/webhook"
)

var (
	port     int
	certFile string
	keyFile  string
)

func init() {
	flag.IntVar(&port, "port", 8443, "Webhook server port")
	flag.StringVar(&certFile, "cert-file", "/etc/webhook/certs/tls.crt", "TLS certificate file")
	flag.StringVar(&keyFile, "key-file", "/etc/webhook/certs/tls.key", "TLS key file")
	flag.Parse()
}

func main() {
	log.Println("Starting sidecar injector webhook server...")

	// Create webhook server
	webhookServer := webhook.NewServer()

	// Setup HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", webhookServer.ServeHTTP)
	mux.HandleFunc("/health", healthCheck)

	// Configure TLS
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to load TLS certificates: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Create HTTPS server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Listening on port %d...", port)
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
