// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getWebServerEndpoint } from "@/util/endpoints";
import { useCallback, useEffect, useRef, useState } from "react";

interface WakeWordState {
    isListening: boolean;
    isActive: boolean;
    error: string | null;
}

/**
 * Hook for background wake word detection
 * Continuously listens for wake words: "initiate alfred protocol", "hi alfred", "hey alfred"
 * When detected, triggers voice input to start
 */
export function useWakeWord(onWakeWordDetected: () => void) {
    const [state, setState] = useState<WakeWordState>({
        isListening: false,
        isActive: false,
        error: null,
    });

    const wsRef = useRef<WebSocket | null>(null);
    const audioContextRef = useRef<AudioContext | null>(null);
    const mediaStreamRef = useRef<MediaStream | null>(null);
    const processorRef = useRef<ScriptProcessorNode | null>(null);

    const cleanup = useCallback(() => {
        // Stop audio processing
        if (processorRef.current) {
            processorRef.current.disconnect();
            processorRef.current = null;
        }

        if (audioContextRef.current) {
            audioContextRef.current.close();
            audioContextRef.current = null;
        }

        if (mediaStreamRef.current) {
            mediaStreamRef.current.getTracks().forEach((track) => track.stop());
            mediaStreamRef.current = null;
        }

        // Close WebSocket
        if (wsRef.current) {
            wsRef.current.close();
            wsRef.current = null;
        }
    }, []);

    const startListening = useCallback(async () => {
        try {
            console.log("[WakeWord] Starting background wake word detection");
            setState((prev) => ({ ...prev, error: null, isListening: true }));

            // Get WebSocket URL from HTTP server endpoint
            const httpEndpoint = getWebServerEndpoint();
            const wsUrl = httpEndpoint.replace("http://", "ws://").replace("https://", "wss://") + "/api/whisper/ws";

            // Connect WebSocket
            const ws = new WebSocket(wsUrl);
            wsRef.current = ws;

            ws.onopen = async () => {
                console.log("[WakeWord] WebSocket connected");

                try {
                    // Request mic access (16kHz mono for Whisper)
                    const stream = await navigator.mediaDevices.getUserMedia({
                        audio: {
                            channelCount: 1,
                            sampleRate: 16000,
                            echoCancellation: true,
                            noiseSuppression: true,
                        },
                    });
                    mediaStreamRef.current = stream;

                    // Create AudioContext
                    const audioContext = new AudioContext({ sampleRate: 16000 });
                    audioContextRef.current = audioContext;

                    const source = audioContext.createMediaStreamSource(stream);

                    // Use ScriptProcessorNode for audio capture
                    // Send audio every 2 seconds for wake word detection
                    const processor = audioContext.createScriptProcessor(4096, 1, 1);
                    processorRef.current = processor;
                    
                    let audioBuffer: number[] = [];
                    const bufferSize = 16000 * 2; // 2 seconds of audio

                    processor.onaudioprocess = (e) => {
                        if (ws.readyState === WebSocket.OPEN) {
                            const inputData = e.inputBuffer.getChannelData(0);
                            
                            // Accumulate audio
                            audioBuffer.push(...Array.from(inputData));

                            // Send every 2 seconds
                            if (audioBuffer.length >= bufferSize) {
                                try {
                                    ws.send(JSON.stringify(audioBuffer));
                                    audioBuffer = [];
                                } catch (err) {
                                    console.error("[WakeWord] Error sending audio:", err);
                                }
                            }
                        }
                    };

                    source.connect(processor);
                    processor.connect(audioContext.destination);

                    setState((prev) => ({ ...prev, isActive: true }));
                } catch (err) {
                    console.error("[WakeWord] Mic access error:", err);
                    setState((prev) => ({
                        ...prev,
                        error: "Microphone access denied",
                        isListening: false,
                    }));
                    ws.close();
                }
            };

            ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);

                    // Ignore config messages
                    if (message.type === "config") {
                        return;
                    }

                    // Check for wake word
                    if (message.isWakeWord && message.text) {
                        console.log("[WakeWord] Wake word detected:", message.text);
                        onWakeWordDetected();
                    }
                } catch (err) {
                    console.error("[WakeWord] Parse error:", err);
                }
            };

            ws.onerror = (err) => {
                console.error("[WakeWord] WebSocket error:", err);
                setState((prev) => ({ ...prev, error: "Connection error" }));
            };

            ws.onclose = () => {
                console.log("[WakeWord] WebSocket closed");
                setState((prev) => ({ ...prev, isActive: false }));
            };
        } catch (err) {
            console.error("[WakeWord] Start error:", err);
            setState((prev) => ({ ...prev, error: "Failed to start wake word detection", isListening: false }));
        }
    }, [onWakeWordDetected]);

    const stopListening = useCallback(() => {
        console.log("[WakeWord] Stopping wake word detection");
        cleanup();
        setState({ isListening: false, isActive: false, error: null });
    }, [cleanup]);

    // Cleanup on unmount
    useEffect(() => {
        return () => {
            cleanup();
        };
    }, [cleanup]);

    return {
        isListening: state.isListening,
        isActive: state.isActive,
        error: state.error,
        startListening,
        stopListening,
    };
}
