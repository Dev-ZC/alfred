// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package whispercpp

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
    upgrader = websocket.Upgrader{
        ReadBufferSize:  1024,
        WriteBufferSize: 1024,
        CheckOrigin: func(r *http.Request) bool {
            // Allow all origins for WebSocket connections
            // In production, you should validate the origin
            return true
        },
    }

    // Global service instance
    whisperService *WhisperService
    serviceMutex   sync.Mutex
)

// InitWhisperService initializes the global Whisper service
func InitWhisperService(config *Config) error {
    serviceMutex.Lock()
    defer serviceMutex.Unlock()

    if whisperService != nil {
        return nil
    }

    service, err := NewWhisperService(config)
    if err != nil {
        return err
    }

    if err := service.Initialize(); err != nil {
        return err
    }

    whisperService = service
    return nil
}

// GetWhisperService returns the global Whisper service instance
func GetWhisperService() *WhisperService {
    serviceMutex.Lock()
    defer serviceMutex.Unlock()
    return whisperService
}

// WebSocketHandler handles WebSocket connections for real-time audio streaming
func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Whisper] WebSocket connection attempt from %s", r.RemoteAddr)
	service := GetWhisperService()
	if service == nil {
		log.Printf("[Whisper] ERROR: Service not initialized")
		http.Error(w, "Whisper service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Whisper] ERROR: WebSocket upgrade failed: %v", err)
		http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
		return
	}
	log.Printf("[Whisper] WebSocket connection established successfully")

	// Add WebSocket connection to the service
	service.AddWebSocket(conn)
}

// TranscribeHandler handles HTTP POST requests for transcribing audio
func TranscribeHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    service := GetWhisperService()
    if service == nil {
        http.Error(w, "Whisper service not initialized", http.StatusServiceUnavailable)
        return
    }

    // Parse request body
    var request struct {
        AudioData []float32 `json:"audioData"`
    }

    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    // Process audio data
    if err := service.ProcessAudio(request.AudioData); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Wait for the result
    result := <-service.results
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(result)
}

// RegisterRoutes registers the Whisper service HTTP handlers
func RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("/api/whisper/ws", WebSocketHandler)
    mux.HandleFunc("/api/whisper/transcribe", TranscribeHandler)
}
