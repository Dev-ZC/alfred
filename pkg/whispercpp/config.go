// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package whispercpp

// Config holds the configuration for the Whisper service
type Config struct {
    ModelPath    string  `json:"modelPath"`
    Threads      int     `json:"threads"`
    MaxAudioLen  int     `json:"maxAudioLen"` // in seconds
    SampleRate   int     `json:"sampleRate"`
    Language     string  `json:"language"`
    EnableVAD    bool    `json:"enableVAD"`
    VADThreshold float32 `json:"vadThreshold"`
    WakeWord     string  `json:"wakeWord"`
}

// DefaultConfig returns a new configuration with default values
func DefaultConfig() *Config {
    return &Config{
        ModelPath:    "/Users/zakicole/alfred/models/ggml-base.en.bin",
        Threads:      4,
        MaxAudioLen:  30,
        SampleRate:   16000,
        Language:     "en",
        EnableVAD:    true,
        VADThreshold: 0.5,
        WakeWord:     "alfred",
    }
}
