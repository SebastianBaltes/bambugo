import React, { useState, useEffect } from 'react';
import { usePrinter } from './hooks/usePrinter';
import './App.css';

function App() {
  const { data, connected, sendCommand } = usePrinter();
  const [files, setFiles] = useState([]);
  const [uploading, setUploading] = useState(false);

  const backendUrl = `http://${window.location.hostname}:8080`;

  useEffect(() => {
    fetchFiles();
  }, []);

  const fetchFiles = async () => {
    try {
      const res = await fetch(`${backendUrl}/files`);
      if (res.ok) {
        const list = await res.json();
        setFiles(list || []);
      }
    } catch (e) {
      console.error('Fehler beim Laden der Dateien:', e);
    }
  };

  const handleUpload = async (e) => {
    const file = e.target.files[0];
    if (!file) return;

    setUploading(true);
    const formData = new FormData();
    formData.append('file', file);

    try {
      const res = await fetch(`${backendUrl}/upload`, {
        method: 'POST',
        body: formData,
      });
      if (res.ok) {
        alert('Upload erfolgreich!');
        fetchFiles();
      } else {
        alert('Upload fehlgeschlagen.');
      }
    } catch (e) {
      console.error('Upload Fehler:', e);
    } finally {
      setUploading(false);
      e.target.value = null; // Reset input
    }
  };

  const formatTime = (minutes) => {
    if (!minutes || minutes <= 0) return '0m';
    const h = Math.floor(minutes / 60);
    const m = minutes % 60;
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  };

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
            
            {/* Kamera-Stream via go2rtc (WebRTC oder MP4) */}
            <div className="card camera-card">
              <h2>Kamera</h2>
              <div className="camera-feed">
                <video 
                  autoPlay 
                  playsInline 
                  muted 
                  controls
                  src={`http://${window.location.hostname}:1984/api/stream.mp4?src=bambu_cam`} 
                  style={{ width: '100%', borderRadius: '8px', minHeight: '200px', backgroundColor: '#000' }}
                />
              </div>
            </div>

            {/* Steuerung */}
            <div className="card control-card">
              <h2>Druckersteuerung</h2>
              <div className="button-group main-controls">
                {data.gcode_state === 'PAUSE' ? (
                   <button className="btn btn-resume" onClick={() => {
                     if(window.confirm('Druck fortsetzen?')) sendCommand('print_resume')
                   }}>▶️ Weiter</button>
                ) : (
                   <button className="btn btn-pause" onClick={() => {
                     if(window.confirm('Druck pausieren?')) sendCommand('print_pause')
                   }}>⏸️ Pause</button>
                )}
                <button className="btn btn-stop" onClick={() => {
                  if(window.confirm('Druck wirklich komplett abbrechen?')) sendCommand('print_stop')
                }}>⏹️ Stop</button>
              </div>
              <div className="button-group light-controls">
                <button className="btn btn-on" onClick={() => sendCommand('light_on')}>💡 Licht An</button>
                <button className="btn btn-off" onClick={() => sendCommand('light_off')}>🌙 Licht Aus</button>
              </div>
            </div>

            {/* AMS Status */}
            {data.ams && data.ams.ams && data.ams.ams[0] && (
              <div className="card ams-card">
                <h2>AMS - Material</h2>
                <div className="ams-slots">
                  {data.ams.ams[0].tray.map((tray) => (
                    <div 
                      key={tray.id} 
                      className={`ams-slot ${tray.state === 10 || tray.id === data.ams.tray_now ? 'active' : ''}`}
                    >
                      <div 
                        className="ams-color-dot" 
                        style={{ backgroundColor: tray.tray_color ? `#${tray.tray_color.substring(0, 6)}` : '#ccc' }}
                      ></div>
                      <div className="ams-info">
                        <span className="ams-type">{tray.tray_type || 'Leer'}</span>
                        <span className="ams-id">Slot {parseInt(tray.id) + 1}</span>
                      </div>
                      {(tray.state === 10 || tray.id === data.ams.tray_now) && (
                        <div className="active-tag">AKTIV</div>
                      )}
                    </div>
                  ))}
                </div>
                <div className="sub-info ams-stats">
                  Feuchtigkeit: {data.ams.ams[0].humidity || '?'} | Temp: {data.ams.ams[0].temp || '?'}°C
                </div>
              </div>
            )}

            <div className="card status-card">
              <h2>Status</h2>
              <div className="value large">{data.gcode_state || 'IDLE'}</div>
              <div className="highlight-info">
                Fortschritt: {data.mc_percent !== undefined ? data.mc_percent : 0}% 
              </div>
              <div className="highlight-info">
                Restzeit: {formatTime(data.mc_remaining_time)}
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

            {/* Dateien & Upload */}
            <div className="card files-card">
              <h2>Dateien (SD-Karte)</h2>
              <div className="file-list">
                {files.length === 0 ? (
                  <p className="no-files">Keine GCode-Dateien gefunden.</p>
                ) : (
                  files.map((f, i) => (
                    <div key={i} className="file-item">
                      📄 {f}
                    </div>
                  ))
                )}
              </div>
              <div className="upload-section">
                <label className="btn btn-upload">
                  {uploading ? 'Hochladen...' : '➕ Datei hochladen'}
                  <input 
                    type="file" 
                    accept=".gcode.3mf" 
                    onChange={handleUpload} 
                    disabled={uploading}
                    style={{ display: 'none' }}
                  />
                </label>
              </div>
            </div>

          </div>
        )}
      </main>
    </div>
  );
}

export default App;
