import { createContext, useContext, useState, useEffect, useRef, ReactNode } from "react";

interface WebSocketState {
  safetyState: any;
  autoExposure: any;
  connected: boolean;
}

const WebSocketContext = createContext<WebSocketState>({
  safetyState: null, autoExposure: null, connected: false
});

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const [safetyState, setSafetyState] = useState<any>(null);
  const [autoExposure, setAutoExposure] = useState<any>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    function connect() {
      if (!mountedRef.current) return;
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${protocol}//${window.location.host}/ws`);
      wsRef.current = ws;
      ws.onopen = () => { if (mountedRef.current) setConnected(true); };
      ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          if (msg.type === "safety") setSafetyState(msg.data);
          if (msg.type === "autoexposure") setAutoExposure(msg.data);
        } catch {}
      };
      ws.onclose = () => {
        if (mountedRef.current) {
          setConnected(false);
          setTimeout(connect, 3000);
        }
      };
    }
    connect();
    return () => {
      mountedRef.current = false;
      wsRef.current?.close();
    };
  }, []);

  return (
    <WebSocketContext.Provider value={{ safetyState, autoExposure, connected }}>
      {children}
    </WebSocketContext.Provider>
  );
}

export function useWebSocket() {
  return useContext(WebSocketContext);
}
