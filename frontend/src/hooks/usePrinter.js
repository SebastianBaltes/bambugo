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
          // Bambu Drucker senden oft verschachtelte "print" Objekte in ihren Reports
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
        wsRef.current.onclose = null; // Prevent reconnect loop on unmount
        wsRef.current.close();
      }
    };
  }, [url]);

  return { data, connected };
}
