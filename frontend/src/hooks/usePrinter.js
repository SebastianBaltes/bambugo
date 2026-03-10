import { useState, useEffect, useRef } from 'react';

export function usePrinter(url = `ws://${window.location.hostname}:8080/ws`) {
  const [data, setData] = useState(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef(null);

  useEffect(() => {
    const connect = () => {
      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = () => {
        setConnected(true);
        console.log('[WS] Verbunden mit BambuGo Backend');
      };

      ws.onclose = () => {
        setConnected(false);
        console.log('[WS] Verbindung verloren. Reconnect in 3s...');
        setTimeout(connect, 3000);
      };

      ws.onmessage = (event) => {
        try {
          const payload = JSON.parse(event.data);
          if (payload.print) {
            setData((prev) => ({ ...prev, ...payload.print }));
          }
        } catch (e) {
          console.error('[WS] Parse-Fehler:', e);
        }
      };
    };

    connect();

    return () => {
      if (wsRef.current) {
        wsRef.current.onclose = null; 
        wsRef.current.close();
      }
    };
  }, [url]);

  const sendCommand = (cmd) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(cmd);
    }
  };

  return { data, connected, sendCommand };
}
