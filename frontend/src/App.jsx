import React from 'react';
import { usePrinter } from './hooks/usePrinter';
import './App.css';

function App() {
  const { data, connected } = usePrinter();

  return (
    <div className="dashboard-container">
      <header className="header">
        <h1>BambuGo</h1>
        <div className="status-indicator">
          <span className={`dot ${connected ? 'online' : 'offline'}`}></span>
          {connected ? 'Verbunden' : 'Offline'}
        </div>
      </header>

      <main className="content">
        {!data ? (
          <div className="loading">Warte auf Drucker-Daten...</div>
        ) : (
          <div className="grid">
            
            {/* Kamera-Stream */}
            <div className="card camera-card">
              <h2>Kamera</h2>
              <div className="camera-feed">
                <img 
                  src={`http://${window.location.hostname}:8080/stream`} 
                  alt="Bambu P1S Live Stream" 
                  onError={(e) => { e.target.style.display = 'none'; }} 
                />
              </div>
            </div>

            <div className="card status-card">
              <h2>Status</h2>
              <div className="value large">{data.gcode_state || 'IDLE'}</div>
              <div className="sub-info">
                Fortschritt: {data.mc_percent !== undefined ? data.mc_percent : 0}% 
              </div>
              <div className="sub-info">
                Restzeit: {data.mc_remaining_time !== undefined ? data.mc_remaining_time : 0} min
              </div>
            </div>

            <div className="card temp-card">
              <h2>Hotend / Nozzle</h2>
              <div className="value">
                {data.nozzle_temper !== undefined ? data.nozzle_temper : '--'}°C
              </div>
              <div className="sub-info">
                Ziel: {data.nozzle_target_temper !== undefined ? data.nozzle_target_temper : '--'}°C
              </div>
            </div>

            <div className="card temp-card">
              <h2>Heizbett</h2>
              <div className="value">
                {data.bed_temper !== undefined ? data.bed_temper : '--'}°C
              </div>
              <div className="sub-info">
                Ziel: {data.bed_target_temper !== undefined ? data.bed_target_temper : '--'}°C
              </div>
            </div>

            <div className="card fan-card">
              <h2>Lüfter</h2>
              <div className="value">
                {data.cooling_fan_speed !== undefined ? Math.round((data.cooling_fan_speed / 2.55)) : 0}%
              </div>
            </div>

          </div>
        )}
      </main>
    </div>
  );
}

export default App;
