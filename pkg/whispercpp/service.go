// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package whispercpp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// TranscriptionResult represents a single transcription result
type TranscriptionResult struct {
	Text       string  `json:"text"`
	IsFinal    bool    `json:"isFinal"`
	IsWakeWord bool    `json:"isWakeWord"`
	Confidence float64 `json:"confidence,omitempty"`
}

// WhisperService handles audio transcription using Whisper.cpp
type WhisperService struct {
	config      *Config
	model       *Model
	mu          sync.Mutex

	audioQueue  chan []float32
	results     chan TranscriptionResult
	cancelFunc  context.CancelFunc

	running     bool
	connections map[*websocket.Conn]bool
	connMutex   sync.RWMutex
}

// NewWhisperService creates a new Whisper service instance
func NewWhisperService(cfg *Config) (*WhisperService, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Resolve model path: support remote URLs and local files
	resolvedModelPath := cfg.ModelPath
	if !strings.HasPrefix(resolvedModelPath, "http://") && !strings.HasPrefix(resolvedModelPath, "https://") {
		// Local file path - use smart resolution
		resolvedModelPath = resolveWhisperPath(cfg.ModelPath, "models/ggml-base.en.bin")
		
		// Final existence check
		if _, err := os.Stat(resolvedModelPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("model file not found: %s", resolvedModelPath)
		}
		
		log.Printf("[Whisper] Using model: %s", resolvedModelPath)
	}

	// Initialize our wrapper Model (uses high-level bindings)
	// Use resolved path in a copied config
	cfg2 := *cfg
	cfg2.ModelPath = resolvedModelPath
	model, err := NewModel(&cfg2)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize whisper model: %w", err)
	}

	return &WhisperService{
		config:      cfg,
		model:       model,
		audioQueue:  make(chan []float32, 1000),
		results:     make(chan TranscriptionResult, 100),
		connections: make(map[*websocket.Conn]bool),
	}, nil
}

// resolveWhisperPath resolves paths for both dev and production environments
func resolveWhisperPath(configPath, fallbackRelPath string) string {
	// If absolute path exists, use it (dev mode)
	if filepath.IsAbs(configPath) {
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	// Try to find resources directory (production mode)
	exePath, err := os.Executable()
	if err == nil {
		// macOS production: /Applications/Wave.app/Contents/MacOS/Wave -> ../../Resources/
		resourcesDir := filepath.Join(filepath.Dir(exePath), "..", "Resources")
		prodPath := filepath.Join(resourcesDir, fallbackRelPath)
		if _, err := os.Stat(prodPath); err == nil {
			absPath, _ := filepath.Abs(prodPath)
			return absPath
		}
		
		// Try legacy packaged path: .../app.asar.unpacked/dist/bin -> ../../../models/
		legacyPath := filepath.Join(filepath.Dir(exePath), "..", "..", "..", "models", filepath.Base(fallbackRelPath))
		if _, err := os.Stat(legacyPath); err == nil {
			absPath, _ := filepath.Abs(legacyPath)
			return absPath
		}
	}

	// Fallback: return original path
	return configPath
}

// Initialize initializes the Whisper service
func (s *WhisperService) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.running = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFunc = cancel

	// Start processing audio in background
	go s.processAudio(ctx)

	// Start broadcasting results to WebSocket clients
	go s.broadcastResults(ctx)

	return nil
}

// broadcastResults reads from results channel and broadcasts to all connected clients
func (s *WhisperService) broadcastResults(ctx context.Context) {
	log.Printf("[Whisper] broadcastResults started")
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-s.results:
			log.Printf("[Whisper] Broadcasting transcription result: %s", result.Text)
			s.BroadcastResult(result)
		}
	}
}

// ProcessAudio processes audio data from the queue
func (s *WhisperService) ProcessAudio(audioData []float32) error {
	if !s.running {
		return fmt.Errorf("service not running")
	}

	select {
	case s.audioQueue <- audioData:
		return nil
	default:
		return fmt.Errorf("audio queue full")
	}
}

// processAudio runs in a background goroutine to process audio chunks
func (s *WhisperService) processAudio(ctx context.Context) {
	var audioBuffer []float32
	// Process every 3 seconds of audio for more responsive transcription
	bufferSize := s.config.SampleRate * 3 // 3 seconds of audio
	minBufferSize := s.config.SampleRate / 2 // 0.5 seconds minimum

	log.Printf("[Whisper] processAudio started, will transcribe every %d samples (3 sec)", bufferSize)

	for {
		select {
		case <-ctx.Done():
			return

		case data := <-s.audioQueue:
			// Append new audio data to buffer
			audioBuffer = append(audioBuffer, data...)

			// Process if we have enough data (3 seconds)
			if len(audioBuffer) >= bufferSize {
				log.Printf("[Whisper] Buffer full (%d samples), transcribing...", len(audioBuffer))
				go s.transcribeAudio(audioBuffer)
				audioBuffer = nil
			}

		case <-time.After(500 * time.Millisecond):
			// Process any remaining audio in the buffer (after 500ms of silence)
			if len(audioBuffer) >= minBufferSize {
				log.Printf("[Whisper] Timeout reached with %d samples, transcribing...", len(audioBuffer))
				go s.transcribeAudio(audioBuffer)
				audioBuffer = nil
			}
		}
	}
}

// transcribeAudio performs the actual transcription
func (s *WhisperService) transcribeAudio(audioData []float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.model == nil {
		log.Println("[Whisper] model not initialized")
		return
	}

	text, err := s.model.Transcribe(audioData)
	if err != nil {
		log.Printf("[Whisper] transcription error: %v", err)
		return
	}

	// Check for wake words
	lowerText := strings.ToLower(strings.TrimSpace(text))
	isWakeWord := strings.Contains(lowerText, "initiate alfred protocol") ||
		strings.Contains(lowerText, "hi alfred") ||
		strings.Contains(lowerText, "hey alfred")

	if isWakeWord {
		log.Printf("[Whisper] Wake word detected: %s", text)
	}

	result := TranscriptionResult{
		Text:       text,
		IsFinal:    true,
		IsWakeWord: isWakeWord,
	}
	s.results <- result
}

// AddWebSocket adds a new WebSocket connection to broadcast transcriptions to
func (s *WhisperService) AddWebSocket(conn *websocket.Conn) {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	s.connections[conn] = true

	// Start a goroutine to handle this connection
	go s.handleWebSocket(conn)
}

// RemoveWebSocket removes a WebSocket connection
func (s *WhisperService) RemoveWebSocket(conn *websocket.Conn) {
	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	delete(s.connections, conn)
}

// handleWebSocket handles a single WebSocket connection
func (s *WhisperService) handleWebSocket(conn *websocket.Conn) {
	defer func() {
		s.RemoveWebSocket(conn)
		conn.Close()
	}()

	// Send initial config
	configMsg := map[string]interface{}{
		"type": "config",
		"config": s.config,
	}
	if err := conn.WriteJSON(configMsg); err != nil {
		log.Printf("error sending config: %v", err)
		return
	}

	// Process incoming audio data
	messageCount := 0
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Whisper] websocket error: %v", err)
			} else {
				log.Printf("[Whisper] websocket closed normally")
			}
			return
		}

		if messageCount == 0 {
			log.Printf("[Whisper] Received first audio message, size: %d bytes", len(message))
		}
		messageCount++

		// Handle audio data
		var audioData []float32
		if err := json.Unmarshal(message, &audioData); err != nil {
			log.Printf("[Whisper] error unmarshaling audio data (message %d): %v, first 100 bytes: %s", messageCount, err, string(message[:min(100, len(message))]))
			continue
		}

		if messageCount == 1 {
			log.Printf("[Whisper] Successfully unmarshaled first audio chunk, length: %d samples", len(audioData))
		}

		// Process the audio
		if err := s.ProcessAudio(audioData); err != nil {
			log.Printf("[Whisper] error processing audio: %v", err)
		}
	}
}

// BroadcastResult sends a transcription result to all connected clients
func (s *WhisperService) BroadcastResult(result TranscriptionResult) {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	for conn := range s.connections {
		if err := conn.WriteJSON(result); err != nil {
			log.Printf("error sending result to client: %v", err)
			conn.Close()
			delete(s.connections, conn)
		}
	}
}

// Close cleans up resources
func (s *WhisperService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancelFunc != nil {
		s.cancelFunc()
	}

	s.running = false

	// Close all WebSocket connections
	s.connMutex.Lock()
	for conn := range s.connections {
		conn.Close()
		delete(s.connections, conn)
	}
	s.connMutex.Unlock()

	// Close channels
	close(s.audioQueue)
	close(s.results)

	// Free Whisper resources
	if s.model != nil {
		s.model.Close()
		s.model = nil
	}

	return nil
}
