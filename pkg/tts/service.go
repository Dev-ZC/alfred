// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package tts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	globalService *TTSService
	serviceMu     sync.Mutex
)

// TTSService handles text-to-speech using Piper
type TTSService struct {
	config *Config
	mu     sync.Mutex
}

// InitTTSService initializes the global TTS service
func InitTTSService(cfg *Config) error {
	serviceMu.Lock()
	defer serviceMu.Unlock()

	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Smart path resolution for both dev and production
	cfg.PiperPath = resolveTTSPath(cfg.PiperPath, "bin/piper")
	cfg.ModelPath = resolveTTSPath(cfg.ModelPath, "models/tts/en_GB-northern_english_male-medium.onnx")

	// Verify piper executable exists
	if _, err := os.Stat(cfg.PiperPath); os.IsNotExist(err) {
		return fmt.Errorf("piper executable not found at: %s", cfg.PiperPath)
	}

	// Verify model file exists
	if _, err := os.Stat(cfg.ModelPath); os.IsNotExist(err) {
		return fmt.Errorf("model file not found at: %s", cfg.ModelPath)
	}

	globalService = &TTSService{
		config: cfg,
	}

	log.Printf("[TTS] Service initialized with piper: %s, model: %s", cfg.PiperPath, cfg.ModelPath)
	return nil
}

// resolveTTSPath resolves paths for both dev and production environments
func resolveTTSPath(configPath, fallbackRelPath string) string {
	// If absolute path exists, use it (dev mode)
	if filepath.IsAbs(configPath) {
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	// Try to find resources directory (production mode)
	// In production, resources are bundled relative to the executable
	exePath, err := os.Executable()
	if err == nil {
		// macOS production: /Applications/Wave.app/Contents/MacOS/Wave -> ../../Resources/
		resourcesDir := filepath.Join(filepath.Dir(exePath), "..", "Resources")
		prodPath := filepath.Join(resourcesDir, fallbackRelPath)
		if _, err := os.Stat(prodPath); err == nil {
			absPath, _ := filepath.Abs(prodPath)
			return absPath
		}
	}

	// Fallback: return original path
	return configPath
}

// GetTTSService returns the global TTS service
func GetTTSService() *TTSService {
	serviceMu.Lock()
	defer serviceMu.Unlock()
	return globalService
}

// setEspeakEnv sets the ESPEAK_DATA_PATH environment variable for the command
func (s *TTSService) setEspeakEnv(cmd *exec.Cmd) {
	// Calculate espeak-ng data path
	// In dev: alfred/bin/piper -> alfred/third_party/piper/build_go/pi/share/espeak-ng-data
	// In prod: resources/bin/piper -> resources/espeak-ng-data
	
	piperDir := filepath.Dir(s.config.PiperPath)
	
	// Try production path first (relative to bin/)
	espeakDataPath := filepath.Join(piperDir, "..", "espeak-ng-data")
	
	// If that doesn't exist, try dev path
	if _, err := os.Stat(espeakDataPath); os.IsNotExist(err) {
		espeakDataPath = filepath.Join(piperDir, "..", "third_party", "piper", "build_go", "pi", "share", "espeak-ng-data")
	}
	
	// Make absolute
	if absPath, err := filepath.Abs(espeakDataPath); err == nil {
		espeakDataPath = absPath
	}
	
	log.Printf("[TTS] Using ESPEAK_DATA_PATH: %s", espeakDataPath)
	cmd.Env = append(os.Environ(), "ESPEAK_DATA_PATH="+espeakDataPath)
}

// SynthesizeToFile synthesizes text to a WAV file
func (s *TTSService) SynthesizeToFile(ctx context.Context, text string, outputPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create command: echo "text" | piper --model model.onnx --output_file output.wav --length-scale X
	cmd := exec.CommandContext(ctx, s.config.PiperPath,
		"--model", s.config.ModelPath,
		"--output_file", outputPath,
		"--length-scale", fmt.Sprintf("%.2f", s.config.LengthScale),
	)

	// Set espeak-ng data path
	s.setEspeakEnv(cmd)

	// Pipe text to stdin
	cmd.Stdin = bytes.NewBufferString(text)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("piper command failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// Synthesize synthesizes text and returns the audio data as bytes
func (s *TTSService) Synthesize(ctx context.Context, text string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create command: echo "text" | piper --model model.onnx --output-raw --length-scale X
	cmd := exec.CommandContext(ctx, s.config.PiperPath,
		"--model", s.config.ModelPath,
		"--output-raw",
		"--length-scale", fmt.Sprintf("%.2f", s.config.LengthScale),
	)

	// Set espeak-ng data path
	s.setEspeakEnv(cmd)

	// Pipe text to stdin
	cmd.Stdin = bytes.NewBufferString(text)
	
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("piper command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// SynthesizeStream synthesizes text and streams audio data
func (s *TTSService) SynthesizeStream(ctx context.Context, text string, writer io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create command: echo "text" | piper --model model.onnx --output-raw --length-scale X
	cmd := exec.CommandContext(ctx, s.config.PiperPath,
		"--model", s.config.ModelPath,
		"--output-raw",
		"--length-scale", fmt.Sprintf("%.2f", s.config.LengthScale),
	)

	// Set espeak-ng data path
	s.setEspeakEnv(cmd)

	// Pipe text to stdin
	cmd.Stdin = bytes.NewBufferString(text)
	cmd.Stdout = writer
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("piper command failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// ExtractVerbalResponse extracts text within [[VERBAL]] tags, or returns the first sentence if no tags
func ExtractVerbalResponse(text string) string {
	// Try to extract [[VERBAL]] tag content
	re := regexp.MustCompile(`(?s)\[\[VERBAL\]\](.*?)\[\[/VERBAL\]\]`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: return first 5 sentences (or until first blank line)
	// Split by double newline first (paragraph break)
	paragraphs := strings.Split(text, "\n\n")
	firstParagraph := paragraphs[0]
	
	// If first paragraph is reasonable length, use it
	if len(firstParagraph) < 1000 {
		return strings.TrimSpace(firstParagraph)
	}
	
	// Otherwise take first 5 sentences
	sentences := regexp.MustCompile(`[.!?]+\s+`).Split(firstParagraph, 6)
	if len(sentences) > 0 {
		count := 5
		if len(sentences) < count {
			count = len(sentences)
		}
		result := strings.Join(sentences[:count], ". ")
		return strings.TrimSpace(result)
	}

	return text
}
