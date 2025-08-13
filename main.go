package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type ServerState struct {
	mu        sync.RWMutex
	isHealthy bool
	isReady   bool
}

func (s *ServerState) SetHealth(status bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isHealthy = status
}

func (s *ServerState) IsHealthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isHealthy
}

func (s *ServerState) SetReady(status bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isReady = status
}

func (s *ServerState) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isReady
}

func NewServerState() *ServerState {
	return &ServerState{
		isHealthy: true,
		isReady:   true,
	}
}

func getStartupDelay() time.Duration {
	delayFlag := flag.String("t", "", "Startup delay duration(e.g., '30s', '2m'")
	flag.Parse()

	delayStr := "120s"

	if val, ok := os.LookupEnv("START_TIME"); ok {
		delayStr = val
	}

	if *delayFlag != "" {
		delayStr = *delayFlag
	}

	log.Printf("Parsing startup delay: %s", delayStr)
	duration, err := time.ParseDuration(delayStr)
	if err != nil {
		log.Fatalf("Invalid format for startup delay '%s'. Error: %v. Please use format like '30s', '5m', '1h'.", delayStr, err)
	}

	return duration
}

func main() {
	state := NewServerState()

	startupDelay := getStartupDelay()
	if startupDelay > 0 {
		log.Printf("Waiting %s before starting the server...", startupDelay)
		time.Sleep(startupDelay)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthy", healthHandler(state))
	mux.HandleFunc("/ready", readyHandler(state))
	mux.HandleFunc("/debug/", debugHandler(state))

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Printf("Server is starting on port 8080...")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Could not listen on port 8080: %v", err)
		}
	}()
	log.Printf("Server started.")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown signal received, starting graceful shutdown...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exiting.")
}

func healthHandler(s *ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.IsHealthy() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "HEALTHY")
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "UNHEALTHY")
		}
	}
}

func readyHandler(s *ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.IsReady() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "READY")
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "NOREADY")
		}
	}
}

func debugHandler(s *ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Path[len("/debug/"):]

		switch action {
		case "healthy":
			s.SetHealth(true)
			log.Println("State changed: /healthy will now return 200")
			fmt.Fprintln(w, "Health status set to HEALTHY (200 OK)")
		case "unhealthy":
			s.SetHealth(false)
			log.Println("State changed: /healthy will now return 500")
			fmt.Fprintln(w, "Health status set to UNHEALTHY (500 Internal Server Error)")
		case "ready":
			s.SetReady(true)
			log.Println("State changed: /ready will now return 200")
			fmt.Fprintln(w, "Ready status set to READY (200 OK)")
		case "noready":
			s.SetReady(false)
			log.Println("State changed: /ready will now return 500")
			fmt.Fprintln(w, "Ready status set to NOREADY (500 Internal Server Error)")
		default:
			http.NotFound(w, r)
		}
	}
}
