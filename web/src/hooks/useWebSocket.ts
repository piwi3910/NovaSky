import { useState, useEffect, useRef } from "react";

interface SafetyState { id: string; state: string; imagingQuality: string; reason: string | null; evaluatedAt: string; }
interface AutoExposureState { mode: string; sunAltitude: number; currentExposureMs: number; currentGain: number; lastMedianAdu: number; targetAdu: number; phase: string; }

export function useWebSocket() {
  const [safetyState, setSafetyState] = useState<SafetyState | null>(null);
  const [autoExposure, setAutoExposure] = useState<AutoExposureState | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    function connect() {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws`);
      wsRef.current = ws;
      ws.onopen = () => setConnected(true);
      ws.onclose = () => { setConnected(false); setTimeout(connect, 3000); };
      ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        if (msg.type === "safety-state") setSafetyState(msg.data);
        if (msg.type === "autoexposure-state") setAutoExposure(msg.data);
      };
    }
    connect();
    return () => { wsRef.current?.close(); };
  }, []);

  return { safetyState, autoExposure, connected };
}
