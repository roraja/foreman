package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/anthropic/foreman/internal/orchestrator"
)

// Server is the HTTP server for the Foreman web UI and API.
type Server struct {
	orch      *orchestrator.Orchestrator
	password  string
	apiToken  string
	startTime time.Time
	mux       *http.ServeMux
}

// New creates a new Server.
func New(orch *orchestrator.Orchestrator, password, apiToken string) *Server {
	s := &Server{
		orch:      orch,
		password:  password,
		apiToken:  apiToken,
		startTime: time.Now(),
	}
	s.setupRoutes()
	return s
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) setupRoutes() {
	s.mux = http.NewServeMux()

	// API routes
	s.mux.HandleFunc("/api/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/services", s.requireAuth(s.handleListServices))
	s.mux.HandleFunc("/api/services/start-all", s.requireAuth(s.handleStartAll))
	s.mux.HandleFunc("/api/services/stop-all", s.requireAuth(s.handleStopAll))
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/config/reload", s.requireAuth(s.handleReloadConfig))

	// Service-specific routes (uses path parsing)
	s.mux.HandleFunc("/api/service/", s.requireAuth(s.handleServiceAction))

	// WebSocket routes
	s.mux.HandleFunc("/ws/logs/", s.requireAuth(s.handleWSLogs))
	s.mux.HandleFunc("/ws/stdin/", s.requireAuth(s.handleWSStdin))

	// Frontend static files
	s.mux.HandleFunc("/", s.handleFrontend)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Password != s.password {
		log.Printf("failed login attempt from %s", r.RemoteAddr)
		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	// Set auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "foreman_auth",
		Value:    s.password, // Simple auth — in production, use a signed token
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400 * 7, // 7 days
	})

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check API token
		if s.apiToken != "" {
			auth := r.Header.Get("Authorization")
			if auth == "Bearer "+s.apiToken {
				next(w, r)
				return
			}
		}

		// Check cookie
		cookie, err := r.Cookie("foreman_auth")
		if err == nil && cookie.Value == s.password {
			next(w, r)
			return
		}

		// If requesting the UI (not API), serve the login page
		if !isAPIRequest(r) {
			next(w, r) // Frontend handles auth state
			return
		}

		jsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	log.Printf("API: listing services (remote: %s)", r.RemoteAddr)
	services := s.orch.ListServices()
	log.Printf("API: returning %d services", len(services))
	jsonResponse(w, http.StatusOK, services)
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/service/{id}/{action}
	path := r.URL.Path[len("/api/service/"):]
	var serviceID, action string

	// Find the last "/" for the action
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			serviceID = path[:i]
			action = path[i+1:]
			break
		}
	}

	if serviceID == "" || action == "" {
		http.Error(w, "invalid path: expected /api/service/{id}/{action}", http.StatusBadRequest)
		return
	}

	switch action {
	case "start":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go func() {
			if err := s.orch.StartService(serviceID); err != nil {
				log.Printf("API: start %s failed: %v", serviceID, err)
			} else {
				log.Printf("API: start %s completed", serviceID)
			}
		}()
		jsonResponse(w, http.StatusAccepted, map[string]string{"status": "starting"})

	case "stop":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go func() {
			if err := s.orch.StopService(serviceID); err != nil {
				log.Printf("API: stop %s failed: %v", serviceID, err)
			} else {
				log.Printf("API: stop %s completed", serviceID)
			}
		}()
		jsonResponse(w, http.StatusAccepted, map[string]string{"status": "stopping"})

	case "restart":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go func() {
			if err := s.orch.RestartService(serviceID); err != nil {
				log.Printf("API: restart %s failed: %v", serviceID, err)
			} else {
				log.Printf("API: restart %s completed", serviceID)
			}
		}()
		jsonResponse(w, http.StatusAccepted, map[string]string{"status": "restarting"})

	case "build":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		log.Printf("API: build requested for %s", serviceID)
		go func() {
			if err := s.orch.BuildService(serviceID); err != nil {
				log.Printf("API: build failed for %s: %v", serviceID, err)
			} else {
				log.Printf("API: build completed for %s", serviceID)
			}
		}()
		jsonResponse(w, http.StatusOK, map[string]string{"status": "building"})

	case "logs":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		n := 100
		if lines := r.URL.Query().Get("lines"); lines != "" {
			if parsed, err := strconv.Atoi(lines); err == nil {
				n = parsed
			}
		}
		logs, err := s.orch.GetLogs(serviceID, n)
		if err != nil {
			jsonResponse(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, logs)

	case "env":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		env, err := s.orch.GetEnv(serviceID)
		if err != nil {
			jsonResponse(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"variables": env})

	case "info":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		info, err := s.orch.GetServiceInfo(serviceID)
		if err != nil {
			jsonResponse(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		jsonResponse(w, http.StatusOK, info)

	default:
		http.Error(w, fmt.Sprintf("unknown action: %s", action), http.StatusBadRequest)
	}
}

func (s *Server) handleStartAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go func() {
		services := s.orch.ListServices()
		for _, svc := range services {
			if svc.Status == "stopped" || svc.Status == "crashed" {
				if err := s.orch.StartService(svc.ID); err != nil {
					log.Printf("API: start-all: failed to start %s: %v", svc.ID, err)
				} else {
					log.Printf("API: start-all: started %s", svc.ID)
				}
			}
		}
		log.Printf("API: start-all completed")
	}()
	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "starting"})
}

func (s *Server) handleStopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	go func() {
		s.orch.StopAll()
		log.Printf("API: stop-all completed")
	}()
	jsonResponse(w, http.StatusAccepted, map[string]string{"status": "stopping"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)
	jsonResponse(w, http.StatusOK, map[string]string{
		"status": "ok",
		"uptime": uptime.String(),
	})
}

func (s *Server) handleReloadConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	added, removed, err := s.orch.ReloadConfig()
	if err != nil {
		log.Printf("API: config reload failed: %v", err)
		jsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("API: config reloaded (added: %v, removed: %v)", added, removed)
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"added":   added,
		"removed": removed,
	})
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func isAPIRequest(r *http.Request) bool {
	return len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" ||
		len(r.URL.Path) >= 3 && r.URL.Path[:3] == "/ws"
}
