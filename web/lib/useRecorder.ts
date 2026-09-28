"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const MAX_MS = 15_000;
// First supported wins: Chrome/Android record webm/opus, Safari/iOS record mp4 (AAC).
const MIME_TYPES = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4"];

export type Recording = { blob: Blob; ms: number };

/**
 * Microphone recorder for one short take. `start()` asks for the mic on first use; `stop()`
 * resolves with the take (or null if nothing was captured) and releases the microphone.
 * Recording stops on its own after 15 seconds and calls `onAutoStop` with the take.
 */
export function useRecorder(onAutoStop: (take: Recording | null) => void) {
  const [recording, setRecording] = useState(false);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const finishRef = useRef<Promise<Recording | null> | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const autoStopRef = useRef(onAutoStop);
  useEffect(() => {
    autoStopRef.current = onAutoStop;
  }, [onAutoStop]);

  const stop = useCallback(async (): Promise<Recording | null> => {
    if (timerRef.current) clearTimeout(timerRef.current);
    const recorder = recorderRef.current;
    if (recorder && recorder.state !== "inactive") recorder.stop();
    const take = await finishRef.current;
    recorderRef.current = null;
    finishRef.current = null;
    setRecording(false);
    return take ?? null;
  }, []);

  const start = useCallback(async () => {
    if (recorderRef.current) return;
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") {
      throw new Error("這個瀏覽器不支援錄音。");
    }
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const mimeType = MIME_TYPES.find((t) => MediaRecorder.isTypeSupported(t));
    const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
    const chunks: Blob[] = [];
    const startedAt = Date.now();
    recorder.ondataavailable = (e) => e.data.size > 0 && chunks.push(e.data);
    finishRef.current = new Promise((resolve) => {
      recorder.onstop = () => {
        stream.getTracks().forEach((t) => t.stop()); // release the mic indicator
        const blob = new Blob(chunks, { type: recorder.mimeType || mimeType || "audio/webm" });
        resolve(blob.size > 0 ? { blob, ms: Date.now() - startedAt } : null);
      };
    });
    recorderRef.current = recorder;
    recorder.start();
    setRecording(true);
    timerRef.current = setTimeout(() => stop().then((take) => autoStopRef.current(take)), MAX_MS);
  }, [stop]);

  // Never leave the microphone open when the page goes away.
  useEffect(() => () => void stop(), [stop]);

  return { recording, start, stop };
}
