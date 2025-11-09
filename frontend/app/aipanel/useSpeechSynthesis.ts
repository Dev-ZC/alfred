// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getWebServerEndpoint } from "@/util/endpoints";
import { useCallback, useRef, useState } from "react";

interface SpeechSynthesisState {
    isSpeaking: boolean;
    error: string | null;
}

/**
 * Hook for text-to-speech using Piper TTS
 */
export function useSpeechSynthesis() {
    const [state, setState] = useState<SpeechSynthesisState>({
        isSpeaking: false,
        error: null,
    });

    const audioContextRef = useRef<AudioContext | null>(null);
    const audioSourceRef = useRef<AudioBufferSourceNode | null>(null);

    const stopSpeaking = useCallback(() => {
        if (audioSourceRef.current) {
            try {
                audioSourceRef.current.stop();
            } catch (e) {
                // Already stopped
            }
            audioSourceRef.current = null;
        }
        setState((prev) => ({ ...prev, isSpeaking: false }));
    }, []);

    const speak = useCallback(
        async (text: string, extractVerbal: boolean = true) => {
            try {
                console.log("[TTS] Synthesizing text:", text.substring(0, 100));
                setState({ isSpeaking: true, error: null });

                // Stop any ongoing speech
                stopSpeaking();

                // Get TTS endpoint
                const endpoint = getWebServerEndpoint();
                const response = await fetch(`${endpoint}/api/tts/synthesize`, {
                    method: "POST",
                    headers: {
                        "Content-Type": "application/json",
                    },
                    body: JSON.stringify({
                        text,
                        extractVerbal,
                    }),
                });

                if (!response.ok) {
                    throw new Error(`TTS request failed: ${response.statusText}`);
                }

                // Get raw PCM audio data
                const audioData = await response.arrayBuffer();
                console.log("[TTS] Received audio data:", audioData.byteLength, "bytes");

                // Create or reuse AudioContext
                if (!audioContextRef.current) {
                    audioContextRef.current = new AudioContext({ sampleRate: 22050 });
                }
                const audioContext = audioContextRef.current;

                // Convert PCM (int16) to Float32Array for Web Audio
                const pcmData = new Int16Array(audioData);
                const floatData = new Float32Array(pcmData.length);
                for (let i = 0; i < pcmData.length; i++) {
                    floatData[i] = pcmData[i] / 32768.0; // Convert int16 to float [-1, 1]
                }

                // Create AudioBuffer
                const audioBuffer = audioContext.createBuffer(1, floatData.length, 22050);
                audioBuffer.getChannelData(0).set(floatData);

                // Create and play audio source
                const source = audioContext.createBufferSource();
                source.buffer = audioBuffer;
                source.connect(audioContext.destination);

                source.onended = () => {
                    console.log("[TTS] Playback finished");
                    setState((prev) => ({ ...prev, isSpeaking: false }));
                    audioSourceRef.current = null;
                };

                audioSourceRef.current = source;
                source.start(0);
                console.log("[TTS] Playback started");
            } catch (err) {
                console.error("[TTS] Error:", err);
                setState({ isSpeaking: false, error: err.message || "TTS failed" });
            }
        },
        [stopSpeaking]
    );

    return {
        isSpeaking: state.isSpeaking,
        error: state.error,
        speak,
        stopSpeaking,
    };
}
