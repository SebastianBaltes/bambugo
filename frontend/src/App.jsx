import React, { useState, useEffect } from 'react';
import { usePrinter } from './hooks/usePrinter';
import './App.css';

function App() {
  const { data, connected, sendCommand } = usePrinter();
  const [files, setFiles] = useState([]);
  const [uploading, setUploading] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [config, setConfig] = useState({
    printer_ip: '',
    printer_serial: '',
    printer_access_code: '',
  });

  const backendUrl = `http://${window.location.hostname}:8080`;

  useEffect(() => {
    fetchFiles();
    fetchConfig();
  }, []);

  const fetchConfig = async () => {
    try {
      const res = await fetch(`${backendUrl}/api/config`);
      if (res.ok) {
        const data = await res.json();
        setConfig(data);
      }
    } catch (e) {
      console.error('Fehler beim Laden der Config:', e);
    }
  };

  const saveConfig = async (e) => {
    e.preventDefault();
    try {
      const res = await fetch(`${backendUrl}/api/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config),
      });
      if (res.ok) {
        alert('Konfiguration gespeichert! Backend verbindet neu...');
        setShowSettings(false);
      } else {
        alert('Fehler beim Speichern.');
      }
    } catch (e) {
      console.error('Save Config Error:', e);
    }
  };

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

  const deleteFile = async (filename) => {
    if (!window.confirm(`Datei "${filename}" wirklich löschen?`)) return;
    try {
      await fetch(`${backendUrl}/files/delete?file=${encodeURIComponent(filename)}`);
      fetchFiles();
    } catch (e) {
      console.error('Löschfehler:', e);
    }
  };

  const [selectedAmsSlotIndex, setSelectedAmsSlotIndex] = useState(null);

  const handleSlotSelect = (slotIndex) => {
    setSelectedAmsSlotIndex(slotIndex === selectedAmsSlotIndex ? null : slotIndex);
  };

  const handlePrintClick = (f) => {
    if (window.confirm(`Soll der Druck von "${f}" gestartet werden?`)) {
      const amsData = selectedAmsSlotIndex !== null ? `:${selectedAmsSlotIndex}` : '';
      sendCommand(`print_file:${f}${amsData}`);
    }
  };

  const handleUpload = async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    setUploading(true);
    const formData = new FormData();
    formData.append('file', file);
    try {
      const res = await fetch(`${backendUrl}/upload`, { method: 'POST', body: formData });
      if (res.ok) { alert('Upload erfolgreich!'); fetchFiles(); }
      else alert('Upload fehlgeschlagen.');
    } catch (e) {
      console.error('Upload Fehler:', e);
    } finally {
      setUploading(false);
      e.target.value = null;
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
        <div className="header-actions">
          <button className="btn-icon" onClick={() => setShowSettings(!showSettings)}>⚙️</button>
          <div className="status-indicator">
            <span className={`dot ${connected ? 'online' : 'offline'}`}></span>
            {connected ? 'Verbunden' : 'Offline'}
          </div>
        </div>
      </header>

      <main className="content">
        {showSettings ? (
          <div className="card settings-card">
            <h2>Konfiguration</h2>
            <form onSubmit={saveConfig}>
              <div className="form-group">
                <label>Drucker IP:</label>
                <input 
                  type="text" 
                  value={config.printer_ip} 
                  onChange={(e) => setConfig({...config, printer_ip: e.target.value})} 
                />
              </div>
              <div className="form-group">
                <label>Seriennummer:</label>
                <input 
                  type="text" 
                  value={config.printer_serial} 
                  onChange={(e) => setConfig({...config, printer_serial: e.target.value})} 
                />
              </div>
              <div className="form-group">
                <label>Access Code:</label>
                <input 
                  type="password" 
                  value={config.printer_access_code} 
                  onChange={(e) => setConfig({...config, printer_access_code: e.target.value})} 
                />
              </div>
              <div className="button-group">
                <button type="submit" className="btn btn-on">Speichern</button>
                <button type="button" className="btn btn-off" onClick={() => setShowSettings(false)}>Abbrechen</button>
              </div>
            </form>
          </div>
        ) : !data ? (
          <div className="loading">Warte auf Drucker-Daten...</div>
        ) : (
          <div className="grid">
            
            {/* 1. Kamera-Stream (Desktop: Links, span 9) */}
            <div className="card camera-card">
              <div className="camera-feed">
                <video 
                  autoPlay 
                  playsInline 
                  muted 
                  controls
                  src={`http://${window.location.hostname}:1984/api/stream.mp4?src=bambu_cam`} 
                />
              </div>
            </div>

            {/* 2. Status (Desktop: Sidebar Top) */}
            <div className="card status-card">
              <h2>Status</h2>
              <div className="value large">{data.gcode_state || 'IDLE'}</div>
              <div className="highlight-info">
                {data.mc_percent !== undefined ? data.mc_percent : 0}% 
              </div>
              <div className="highlight-info">
                {formatTime(data.mc_remaining_time)}
              </div>
            </div>

            {/* 3. Steuerung (Desktop: Sidebar) */}
            <div className="card control-card">
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
                <button className="btn btn-on" onClick={() => sendCommand('light_on')}>💡 Licht</button>
                <button className="btn btn-off" onClick={() => sendCommand('light_off')}>🌙 Aus</button>
              </div>
            </div>

            {/* 4. Temperaturen & Lüfter (Desktop: Sidebar) */}
            <div className="card temp-card">
              <h2>Hotend</h2>
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

            {/* 5. AMS (Desktop: Unten Breit) */}
            {data.ams && data.ams.ams && data.ams.ams[0] && (
              <div className="card ams-card">
                <h2>AMS - Material</h2>
                <div className="ams-slots">
                  {data.ams.ams[0].tray.map((tray) => (
                    <div
                      key={tray.id}
                      className={`ams-slot ${tray.state === 10 || tray.id === data.ams.tray_now ? 'active' : ''} ${parseInt(tray.id) === selectedAmsSlotIndex ? 'selected' : ''}`}
                      onClick={() => handleSlotSelect(parseInt(tray.id))}
                      style={{ cursor: 'pointer' }}
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
              </div>
            )}

            {/* 6. Dateien & Upload (Desktop: Unten Breit) */}
            <div className="card files-card">
              <h2>Dateien (SD-Karte)</h2>
              <div className="file-list">
                {files.length === 0 ? (
                  <p className="no-files">Keine GCode-Dateien gefunden.</p>
                ) : (
                  files.map((f, i) => (
                    <div key={i} className="file-item">
                      <span className="file-name">📄 {f}</span>
                      <div className="file-actions">
                        <button
                          className="btn-mini btn-print"
                          onClick={() => handlePrintClick(f)}
                        >
                          Drucken
                        </button>
                        <button className="btn-mini btn-delete" onClick={() => deleteFile(f)}>🗑️</button>
                      </div>
                    </div>
                  ))
                )}
              </div>
              <div className="upload-section">
                <label className="btn btn-upload">
                  {uploading ? 'Hochladen...' : '➕ Datei hochladen'}
                  <input 
                    type="file" 
                    accept=".gcode,.gcode.3mf" 
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
