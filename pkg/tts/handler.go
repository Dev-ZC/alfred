// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// TTSRequest represents a text-to-speech request
type TTSRequest struct {
	Text         string `json:"text"`
	ExtractVerbal bool   `json:"extractVerbal"` // If true, extract [[VERBAL]] tags
}

// SynthesizeHandler handles HTTP POST requests for TTS
func SynthesizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := GetTTSService()
	if service == nil {
		log.Printf("[TTS] ERROR: Service not initialized")
		http.Error(w, "TTS service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Parse request
	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[TTS] ERROR: Failed to parse request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	log.Printf("[TTS] Synthesizing text (length: %d chars)", len(req.Text))

	// Extract verbal response if requested
	textToSpeak := req.Text
	if req.ExtractVerbal {
		textToSpeak = ExtractVerbalResponse(req.Text)
		log.Printf("[TTS] Extracted verbal: %s", textToSpeak)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Synthesize
	log.Printf("[TTS] Starting synthesis for text: %s", textToSpeak)
	audioData, err := service.Synthesize(ctx, textToSpeak)
	if err != nil {
		log.Printf("[TTS] ERROR: Synthesis failed: %v", err)
		http.Error(w, fmt.Sprintf("Synthesis failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[TTS] Synthesis complete, audio size: %d bytes", len(audioData))

	// Return audio data as raw PCM
	w.Header().Set("Content-Type", "audio/pcm")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(audioData)))
	w.WriteHeader(http.StatusOK)
	w.Write(audioData)
}
