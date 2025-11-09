// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getWebServerEndpoint } from "@/util/endpoints";
import { useCallback, useEffect, useRef, useState } from "react";

interface VoiceInputState {
    isListening: boolean;
    isConnecting: boolean;
    transcript: string;
    error: string | null;
}

export function useVoiceInput() {
    const [state, setState] = useState<VoiceInputState>({
        isListening: false,
        isConnecting: false,
        transcript: "",
        error: null,
    });

    const wsRef = useRef<WebSocket | null>(null);
    const audioContextRef = useRef<AudioContext | null>(null);
    const mediaStreamRef = useRef<MediaStream | null>(null);
    const audioWorkletNodeRef = useRef<AudioWorkletNode | null>(null);

    const cleanup = useCallback(() => {
        // Stop audio worklet
        if (audioWorkletNodeRef.current) {
            audioWorkletNodeRef.current.disconnect();
            audioWorkletNodeRef.current = null;
        }

        // Close media stream
        if (mediaStreamRef.current) {
            mediaStreamRef.current.getTracks().forEach((track) => track.stop());
            mediaStreamRef.current = null;
        }

        // Close audio context
        if (audioContextRef.current && audioContextRef.current.state !== "closed") {
            audioContextRef.current.close();
            audioContextRef.current = null;
        }

        // Close WebSocket
        if (wsRef.current && wsRef.current.readyState !== WebSocket.CLOSED) {
            wsRef.current.close();
            wsRef.current = null;
        }

        setState((prev) => ({ ...prev, isListening: false, isConnecting: false }));
    }, []);

    const startListening = useCallback(async () => {
        try {
            setState((prev) => ({ ...prev, isConnecting: true, error: null, transcript: "" }));

            // Get WebSocket URL from HTTP server endpoint
            const httpEndpoint = getWebServerEndpoint();
            const wsUrl = httpEndpoint.replace('http://', 'ws://').replace('https://', 'wss://') + '/api/whisper/ws';
            console.log("[VoiceInput] Connecting to:", wsUrl);

            // Connect WebSocket
            const ws = new WebSocket(wsUrl);
            wsRef.current = ws;

            ws.onopen = async () => {
                console.log("[VoiceInput] WebSocket connected");

                try {
                    // Request mic access (16kHz mono for Whisper)
                    const stream = await navigator.mediaDevices.getUserMedia({
                        audio: {
                            sampleRate: 16000,
                            channelCount: 1,
                            echoCancellation: true,
                            noiseSuppression: true,
                        },
                    });
                    mediaStreamRef.current = stream;

                    // Create AudioContext
                    const audioContext = new AudioContext({ sampleRate: 16000 });
                    audioContextRef.current = audioContext;

                    const source = audioContext.createMediaStreamSource(stream);

                    // Use ScriptProcessorNode (AudioWorklet would be better but more complex)
                    const processor = audioContext.createScriptProcessor(4096, 1, 1);
                    let chunkCount = 0;
                    let lastAudioTime = Date.now();
                    let silenceTimer: NodeJS.Timeout | null = null;
                    
                    processor.onaudioprocess = (e) => {
                        if (ws.readyState === WebSocket.OPEN) {
                            const inputData = e.inputBuffer.getChannelData(0);
                            const audioArray = Array.from(inputData);
                            
                            // Detect silence: check if audio level is below threshold
                            const rms = Math.sqrt(audioArray.reduce((sum, val) => sum + val * val, 0) / audioArray.length);
                            const isSilent = rms < 0.01; // Threshold for silence
                            
                            if (!isSilent) {
                                // Log first non-silent chunk for debugging
                                if (chunkCount === 0) {
                                    console.log("[VoiceInput] Sending first audio chunk, RMS:", rms.toFixed(4));
                                }
                                chunkCount++;
                                lastAudioTime = Date.now();
                                
                                // Send Float32 array as JSON (Whisper backend expects this)
                                try {
                                    ws.send(JSON.stringify(audioArray));
                                } catch (err) {
                                    console.error("[VoiceInput] Error sending audio:", err);
                                }
                                
                                // Reset silence timer
                                if (silenceTimer) {
                                    clearTimeout(silenceTimer);
                                    silenceTimer = null;
                                }
                            } else {
                                // Start silence timer if not already running
                                if (!silenceTimer && chunkCount > 0) {
                                    silenceTimer = setTimeout(() => {
                                        console.log("[VoiceInput] 2 seconds of silence detected, auto-submitting");
                                        // Trigger auto-submit by dispatching custom event
                                        window.dispatchEvent(new CustomEvent("voice-auto-submit"));
                                    }, 2000);
                                }
                            }
                        }
                    };

                    source.connect(processor);
                    processor.connect(audioContext.destination);

                    setState((prev) => ({ ...prev, isListening: true, isConnecting: false }));
                } catch (err) {
                    console.error("[VoiceInput] Mic access error:", err);
                    setState((prev) => ({
                        ...prev,
                        error: "Microphone access denied",
                        isConnecting: false,
                    }));
                    ws.close();
                }
            };

            ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    console.log("[VoiceInput] Received message:", message);
                    
                    // Handle different message types
                    if (message.type === "config") {
                        console.log("[VoiceInput] Received config:", message.config);
                        return; // Just acknowledge the config
                    }
                    
                    // Handle transcription results
                    if (message.text) {
                        console.log("[VoiceInput] Transcription:", message.text);
                        
                        setState((prev) => {
                            const newTranscript = prev.transcript + message.text;
                            
                            // Check for termination phrases
                            const lowerText = newTranscript.toLowerCase();
                            if (lowerText.includes("terminate alfred protocol") || 
                                lowerText.includes("close alfred protocol")) {
                                console.log("[VoiceInput] Termination phrase detected, stopping");
                                setTimeout(() => stopListening(), 100);
                            }
                            
                            return {
                                ...prev,
                                transcript: newTranscript,
                            };
                        });
                    }
                } catch (err) {
                    console.error("[VoiceInput] Parse error:", err);
                }
            };

            ws.onerror = (err) => {
                console.error("[VoiceInput] WebSocket error:", err);
                setState((prev) => ({ ...prev, error: "Connection error", isConnecting: false }));
                cleanup();
            };

            ws.onclose = () => {
                console.log("[VoiceInput] WebSocket closed");
                cleanup();
            };
        } catch (err) {
            console.error("[VoiceInput] Start error:", err);
            setState((prev) => ({ ...prev, error: "Failed to start listening", isConnecting: false }));
            cleanup();
        }
    }, [cleanup]);

    const stopListening = useCallback(() => {
        cleanup();
    }, [cleanup]);

    const clearTranscript = useCallback(() => {
        setState((prev) => ({ ...prev, transcript: "" }));
    }, []);

    // Cleanup on unmount
    useEffect(() => {
        return () => {
            cleanup();
        };
    }, [cleanup]);

    return {
        isListening: state.isListening,
        isConnecting: state.isConnecting,
        transcript: state.transcript,
        error: state.error,
        startListening,
        stopListening,
        clearTranscript,
    };
}
