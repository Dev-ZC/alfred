// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package whispercpp

import (
	"fmt"
	"os"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// Model represents a Whisper model
type Model struct {
	config  *Config
	model   whisper.Model
}

// Segment represents a single transcribed segment with timing information
type Segment struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// NewModel initializes a new Whisper model with the given configuration
func NewModel(config *Config) (*Model, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Verify model file exists
	if _, err := os.Stat(config.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", config.ModelPath)
	}

	// Load the model
	model, err := whisper.New(config.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load whisper model: %w", err)
	}

	return &Model{
		config:  config,
		model:   model,
	}, nil
}

// Transcribe processes audio data and returns the transcribed text
func (m *Model) Transcribe(samples []float32) (string, error) {
	if m.model == nil {
		return "", fmt.Errorf("model not initialized")
	}

	ctx, err := m.model.NewContext()
	if err != nil {
		return "", fmt.Errorf("failed to create whisper context: %w", err)
	}
	// Configure context
	if m.config.Language != "" {
		_ = ctx.SetLanguage(m.config.Language)
	}
	if m.config.Threads > 0 {
		ctx.SetThreads(uint(m.config.Threads))
	}

	if err := ctx.Process(samples, nil, nil, nil); err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	var out string
	for {
		seg, err := ctx.NextSegment()
		if err != nil {
			break
		}
		out += seg.Text + " "
	}
	return out, nil
}

// Close cleans up any resources used by the model
func (m *Model) Close() error {
	if m.model != nil {
		m.model.Close()
		m.model = nil
	}

	return nil
}
