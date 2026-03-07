package server

import (
	"log"
	"net/http"
	"strings"

	"golang.org/x/net/websocket"
)

func (s *Server) handleWSLogs(w http.ResponseWriter, r *http.Request) {
	serviceID := strings.TrimPrefix(r.URL.Path, "/ws/logs/")
	if serviceID == "" {
		http.Error(w, "service ID required", http.StatusBadRequest)
		return
	}

	log.Printf("ws: logs connection opened for service %s (remote: %s)", serviceID, r.RemoteAddr)

	handler := websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		ch, err := s.orch.SubscribeLogs(serviceID)
		if err != nil {
			log.Printf("ws: logs subscribe error for %s: %v", serviceID, err)
			return
		}
		defer s.orch.UnsubscribeLogs(serviceID, ch)

		log.Printf("ws: streaming logs for %s", serviceID)
		for entry := range ch {
			msg := map[string]string{
				"timestamp": entry.Timestamp.Format("15:04:05"),
				"stream":    entry.Stream,
				"line":      entry.Line,
			}
			if err := websocket.JSON.Send(ws, msg); err != nil {
				log.Printf("ws: logs send error for %s: %v (closing)", serviceID, err)
				return
			}
		}
		log.Printf("ws: logs connection closed for service %s", serviceID)
	})

	handler.ServeHTTP(w, r)
}

func (s *Server) handleWSStdin(w http.ResponseWriter, r *http.Request) {
	serviceID := strings.TrimPrefix(r.URL.Path, "/ws/stdin/")
	if serviceID == "" {
		http.Error(w, "service ID required", http.StatusBadRequest)
		return
	}

	log.Printf("ws: stdin connection opened for service %s (remote: %s)", serviceID, r.RemoteAddr)

	handler := websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		for {
			var msg struct {
				Data string `json:"data"`
			}
			if err := websocket.JSON.Receive(ws, &msg); err != nil {
				log.Printf("ws: stdin receive error for %s: %v (closing)", serviceID, err)
				return
			}
			log.Printf("ws: stdin data received for %s (%d bytes)", serviceID, len(msg.Data))
			if err := s.orch.WriteStdin(serviceID, []byte(msg.Data)); err != nil {
				log.Printf("ws: stdin write error for %s: %v", serviceID, err)
				return
			}
		}
	})

	handler.ServeHTTP(w, r)
}

func (s *Server) handleWSCommandLogs(w http.ResponseWriter, r *http.Request) {
	commandID := strings.TrimPrefix(r.URL.Path, "/ws/command/")
	if commandID == "" {
		http.Error(w, "command ID required", http.StatusBadRequest)
		return
	}

	log.Printf("ws: command logs connection opened for %s (remote: %s)", commandID, r.RemoteAddr)

	handler := websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		ch, err := s.orch.SubscribeCommandLogs(commandID)
		if err != nil {
			log.Printf("ws: command logs subscribe error for %s: %v", commandID, err)
			return
		}
		defer s.orch.UnsubscribeCommandLogs(commandID, ch)

		for entry := range ch {
			msg := map[string]string{
				"timestamp": entry.Timestamp.Format("15:04:05"),
				"stream":    entry.Stream,
				"line":      entry.Line,
			}
			if err := websocket.JSON.Send(ws, msg); err != nil {
				log.Printf("ws: command logs send error for %s: %v (closing)", commandID, err)
				return
			}
		}
	})

	handler.ServeHTTP(w, r)
}
