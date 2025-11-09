// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package tts

// Config holds TTS configuration
type Config struct {
	PiperPath   string  `json:"piperPath"`   // Path to piper executable
	ModelPath   string  `json:"modelPath"`   // Path to voice model .onnx file
	SampleRate  int     `json:"sampleRate"`  // Output sample rate
	LengthScale float64 `json:"lengthScale"` // Speech rate (1.0 = normal, >1.0 = slower, <1.0 = faster)
}

// DefaultConfig returns default TTS configuration
func DefaultConfig() *Config {
	// For dev: use absolute paths
	// For prod: these will be overridden by electron-builder bundled resources
	return &Config{
		PiperPath:   "/Users/zakicole/alfred/bin/piper",
		ModelPath:   "/Users/zakicole/alfred/models/tts/en_GB-northern_english_male-medium.onnx",
		SampleRate:  22050,
		LengthScale: 1.15, // Slightly slower than default (1.0)
	}
}
