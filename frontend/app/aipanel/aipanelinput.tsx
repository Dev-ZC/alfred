// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { formatFileSizeError, isAcceptableFile, validateFileSize } from "@/app/aipanel/ai-utils";
import { waveAIHasFocusWithin } from "@/app/aipanel/waveai-focus-utils";
import { type WaveAIModel } from "@/app/aipanel/waveai-model";
import { useVoiceInput } from "./useVoiceInput";
import { useWakeWord } from "./useWakeWord";
import { useSpeechSynthesis } from "./useSpeechSynthesis";
import { cn } from "@/util/util";
import { useAtom, useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useRef, useState } from "react";

interface AIPanelInputProps {
    onSubmit: (e: React.FormEvent) => void;
    status: string;
    model: WaveAIModel;
    ttsEnabled: boolean;
    setTtsEnabled: (enabled: boolean) => void;
    isSpeaking: boolean;
    stopSpeaking: () => void;
}

export interface AIPanelInputRef {
    focus: () => void;
    resize: () => void;
}

export const AIPanelInput = memo(({ onSubmit, status, model, ttsEnabled, setTtsEnabled, isSpeaking, stopSpeaking }: AIPanelInputProps) => {
    const [input, setInput] = useAtom(model.inputAtom);
    const isFocused = useAtomValue(model.isWaveAIFocusedAtom);
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const isPanelOpen = useAtomValue(model.getPanelVisibleAtom());
    const { isListening, isConnecting, transcript, error: voiceError, startListening, stopListening, clearTranscript } = useVoiceInput();

    // Wake word detection (always listening in background)
    const {
        isListening: wakeWordListening,
        isActive: wakeWordActive,
        error: wakeWordError,
        startListening: startWakeWord,
        stopListening: stopWakeWord,
    } = useWakeWord(() => {
        console.log("[AIPanelInput] Wake word detected, starting voice input");
        if (!isListening) {
            startListening();
        }
    });

    const resizeTextarea = useCallback(() => {
        const textarea = textareaRef.current;
        if (!textarea) return;

        textarea.style.height = "auto";
        const scrollHeight = textarea.scrollHeight;
        const maxHeight = 7 * 24;
        textarea.style.height = `${Math.min(scrollHeight, maxHeight)}px`;
    }, []);

    useEffect(() => {
        const inputRefObject: React.RefObject<AIPanelInputRef> = {
            current: {
                focus: () => {
                    textareaRef.current?.focus();
                },
                resize: resizeTextarea,
            },
        };
        model.registerInputRef(inputRefObject);
    }, [model, resizeTextarea]);

    const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
        const isComposing = e.nativeEvent?.isComposing || e.keyCode == 229;
        if (e.key === "Enter" && !e.shiftKey && !isComposing) {
            e.preventDefault();
            onSubmit(e as any);
        }
    };

    const handleFocus = useCallback(() => {
        model.requestWaveAIFocus();
    }, [model]);

    const handleBlur = useCallback((e: React.FocusEvent) => {
        if (e.relatedTarget === null) {
            return;
        }

        if (waveAIHasFocusWithin(e.relatedTarget)) {
            return;
        }

        model.requestNodeFocus();
    }, [model]);

    useEffect(() => {
        resizeTextarea();
    }, [input, resizeTextarea]);

    // Update input with voice transcript
    useEffect(() => {
        if (transcript) {
            setInput(transcript);
        }
    }, [transcript, setInput]);

    // Show voice error if any
    useEffect(() => {
        if (voiceError) {
            model.setError(voiceError);
        }
    }, [voiceError, model]);

    // Handle voice auto-submit after silence
    useEffect(() => {
        const handleAutoSubmit = () => {
            if (isListening && transcript) {
                console.log("[AIPanelInput] Auto-submitting after silence");
                stopListening();
                // Submit the form
                const form = document.querySelector('form');
                if (form) {
                    form.requestSubmit();
                }
            }
        };

        window.addEventListener("voice-auto-submit", handleAutoSubmit);
        return () => window.removeEventListener("voice-auto-submit", handleAutoSubmit);
    }, [isListening, transcript, stopListening]);

    useEffect(() => {
        if (isPanelOpen) {
            resizeTextarea();
        }
    }, [isPanelOpen, resizeTextarea]);

    const handleUploadClick = () => {
        fileInputRef.current?.click();
    };

    const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const files = Array.from(e.target.files || []);
        const acceptableFiles = files.filter(isAcceptableFile);

        for (const file of acceptableFiles) {
            const sizeError = validateFileSize(file);
            if (sizeError) {
                model.setError(formatFileSizeError(sizeError));
                if (e.target) {
                    e.target.value = "";
                }
                return;
            }
            await model.addFile(file);
        }

        if (acceptableFiles.length < files.length) {
            console.warn(`${files.length - acceptableFiles.length} files were rejected due to unsupported file types`);
        }

        if (e.target) {
            e.target.value = "";
        }
    };

    const handleVoiceClick = () => {
        if (isListening) {
            stopListening();
        } else {
            clearTranscript();
            startListening();
        }
    };

    return (
        <div className={cn("border-t", isFocused ? "border-accent/50" : "border-gray-600")}>
            <input
                ref={fileInputRef}
                type="file"
                multiple
                accept="image/*,.pdf,.txt,.md,.js,.jsx,.ts,.tsx,.go,.py,.java,.c,.cpp,.h,.hpp,.html,.css,.scss,.sass,.json,.xml,.yaml,.yml,.sh,.bat,.sql"
                onChange={handleFileChange}
                className="hidden"
            />
            <form onSubmit={onSubmit}>
                <div className="relative">
                    <textarea
                        ref={textareaRef}
                        value={input}
                        onChange={(e) => setInput(e.target.value)}
                        onKeyDown={handleKeyDown}
                        onFocus={handleFocus}
                        onBlur={handleBlur}
                        placeholder={model.inBuilder ? "What would you like to build..." : "Ask Wave AI anything..."}
                        className={cn(
                            "w-full  text-white px-2 py-2 pr-5 focus:outline-none resize-none overflow-auto",
                            isFocused ? "bg-accent-900/50" : "bg-gray-800"
                        )}
                        style={{ fontSize: "13px" }}
                        rows={2}
                    />
                    <button
                        type="button"
                        onClick={handleUploadClick}
                        className={cn(
                            "absolute bottom-6 right-1 w-3.5 h-3.5 transition-colors flex items-center justify-center text-gray-400 hover:text-accent cursor-pointer"
                        )}
                    >
                        <i className="fa fa-paperclip text-xs"></i>
                    </button>
                    <button
                        type="button"
                        onClick={() => (wakeWordListening ? stopWakeWord() : startWakeWord())}
                        className={cn(
                            "absolute bottom-6 right-13 w-3.5 h-3.5 transition-colors flex items-center justify-center cursor-pointer",
                            wakeWordActive ? "text-green-500 hover:text-green-400" : "text-gray-400 hover:text-accent"
                        )}
                        title={wakeWordListening ? "Disable wake word detection" : "Enable wake word detection"}
                    >
                        <i className={cn("fa text-xs", wakeWordActive ? "fa-ear-listen" : "fa-magic")}></i>
                    </button>
                    <button
                        type="button"
                        onClick={() => {
                            if (isSpeaking) {
                                stopSpeaking();
                            } else {
                                setTtsEnabled(!ttsEnabled);
                            }
                        }}
                        className={cn(
                            "absolute bottom-6 right-9 w-3.5 h-3.5 transition-colors flex items-center justify-center cursor-pointer",
                            isSpeaking
                                ? "text-blue-500 hover:text-blue-400"
                                : ttsEnabled
                                  ? "text-blue-500 hover:text-blue-400"
                                  : "text-gray-400 hover:text-accent"
                        )}
                        title={isSpeaking ? "Stop speaking" : ttsEnabled ? "Disable voice responses" : "Enable voice responses"}
                    >
                        {isSpeaking ? (
                            <i className="fa fa-volume-high text-xs animate-pulse"></i>
                        ) : (
                            <i className={cn("fa text-xs", ttsEnabled ? "fa-volume-high" : "fa-volume-xmark")}></i>
                        )}
                    </button>
                    <button
                        type="button"
                        onClick={handleVoiceClick}
                        disabled={isConnecting}
                        className={cn(
                            "absolute bottom-6 right-5 w-3.5 h-3.5 transition-colors flex items-center justify-center cursor-pointer",
                            isListening ? "text-red-500 hover:text-red-400" : "text-gray-400 hover:text-accent",
                            isConnecting && "opacity-50 cursor-wait"
                        )}
                        title={isListening ? "Stop listening" : "Start voice input"}
                    >
                        {isConnecting ? (
                            <i className="fa fa-spinner fa-spin text-xs"></i>
                        ) : (
                            <i className={cn("fa text-xs", isListening ? "fa-stop-circle" : "fa-microphone")}></i>
                        )}
                    </button>
                    <button
                        type="submit"
                        disabled={status !== "ready" || !input.trim()}
                        className={cn(
                            "absolute bottom-2 right-1 w-3.5 h-3.5 transition-colors flex items-center justify-center",
                            status !== "ready" || !input.trim()
                                ? "text-gray-400"
                                : "text-accent/80 hover:text-accent cursor-pointer"
                        )}
                    >
                        {status === "streaming" ? (
                            <i className="fa fa-spinner fa-spin text-xs"></i>
                        ) : (
                            <i className="fa fa-paper-plane text-xs"></i>
                        )}
                    </button>
                </div>
            </form>
        </div>
    );
});

AIPanelInput.displayName = "AIPanelInput";
