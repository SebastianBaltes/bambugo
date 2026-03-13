package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

const (
	broker   = "ssl://192.168.178.55:8883"
	user     = "bblp"
	password = "b4129081"
	topic    = "device/+/report"
	cmdTopic = "device/22E8BJ5C1401719/request"
	ftpsUrl  = "ftps://192.168.178.55:990/"
	port     = ":8080"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients     = make(map[*websocket.Conn]bool)
	clientsMu   sync.Mutex
	mqttClient  mqtt.Client
	seqCounter  uint64

	// Cache für Moonraker Status-Abfragen
	latestData   map[string]any
	latestDataMu sync.RWMutex
)

func main() {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("bambugo-backend").
		SetUsername(user).
		SetPassword(password).
		SetTLSConfig(tlsConfig)

	opts.OnConnect = func(c mqtt.Client) {
		log.Println("[MQTT] Verbunden mit Bambu Drucker!")
		if token := c.Subscribe(topic, 0, messageHandler); token.Wait() && token.Error() != nil {
			log.Printf("[MQTT] Subscribe Fehler: %v\n", token.Error())
		}
	}

	mqttClient = mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[MQTT] Verbindungsfehler: %v", token.Error())
	}

	http.HandleFunc("/ws", wsEndpoint)
	http.HandleFunc("/stream", camHandler)
	http.HandleFunc("/files", filesHandler)
	http.HandleFunc("/upload", uploadHandler)
	
	// Moonraker API Bridge für Orca Slicer
	http.HandleFunc("/", logRequest(rootHandler))
	http.HandleFunc("/api/version", logRequest(octoVersionHandler))
	http.HandleFunc("/printer/info", logRequest(moonrakerInfoHandler))
	http.HandleFunc("/server/files/upload", logRequest(moonrakerUploadHandler))
	http.HandleFunc("/printer/objects/query", logRequest(moonrakerQueryHandler))
	
	log.Printf("[HTTP] Backend läuft auf %s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("[HTTP] ListenAndServe:", err)
	}
}

func logRequest(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HTTP] %s %s\n", r.Method, r.URL.Path)
		h(w, r)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("BambuGo Backend is running"))
}

func octoVersionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"api": "0.1",
		"server": "1.3.10",
		"text": "OctoPrint (BambuGo Bridge)",
	}
	json.NewEncoder(w).Encode(resp)
}

func messageHandler(client mqtt.Client, msg mqtt.Message) {
	payload := msg.Payload()

	// Status im Cache speichern für Moonraker Bridge
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err == nil {
		if printObj, ok := data["print"].(map[string]any); ok {
			latestDataMu.Lock()
			if latestData == nil {
				latestData = make(map[string]any)
			}
			for k, v := range printObj {
				latestData[k] = v
			}
			latestDataMu.Unlock()
		}
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()
	for conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			conn.Close()
			delete(clients, conn)
		}
	}
}

func nextSequenceID() string {
	return strconv.FormatUint(atomic.AddUint64(&seqCounter, 1), 10)
}

func sendLightCommand(mode string) {
	payload := map[string]any{
		"system": map[string]any{
			"sequence_id":  nextSequenceID(),
			"command":      "ledctrl",
			"led_node":     "chamber_light",
			"led_mode":     mode,
			"led_on_time":  0,
			"led_off_time": 0,
			"loop_times":   0,
			"interval_time": 0,
		},
	}
	b, _ := json.Marshal(payload)
	mqttClient.Publish(cmdTopic, 0, false, b)
}

func sendPrintCommand(cmd string) {
	payload := map[string]any{
		"print": map[string]any{
			"sequence_id": nextSequenceID(),
			"command":     cmd,
		},
	}
	b, _ := json.Marshal(payload)
	mqttClient.Publish(cmdTopic, 0, false, b)
}

func wsEndpoint(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			clientsMu.Lock()
			delete(clients, ws)
			clientsMu.Unlock()
			break
		}

		command := string(msg)
		switch command {
		case "light_on":
			log.Println("[CMD] Licht AN")
			sendLightCommand("on")
		case "light_off":
			log.Println("[CMD] Licht AUS")
			sendLightCommand("off")
		case "print_pause":
			log.Println("[CMD] Druck PAUSE")
			sendPrintCommand("pause")
		case "print_resume":
			log.Println("[CMD] Druck RESUME")
			sendPrintCommand("resume")
		case "print_stop":
			log.Println("[CMD] Druck STOP")
			sendPrintCommand("stop")
		default:
			// Check for specialized commands like print_file:filename.gcode.3mf
			if strings.HasPrefix(command, "print_file:") {
				filename := strings.TrimPrefix(command, "print_file:")
				log.Printf("[CMD] Starte Druck für Datei: %s\n", filename)
				
				payload := map[string]any{
					"print": map[string]any{
						"sequence_id":    nextSequenceID(),
						"command":        "project_file",
						"param":          "Metadata/slice_1.gcode",
						"subtask_name":   filename,
						"url":            filename,
						"bed_type":       "auto",
						"timelapse":      true,
						"bed_leveling":   true,
						"flow_cali":      true,
						"vibration_cali": true,
						"layer_inspect":  true,
						"ams_mapping":    []int{-1, -1, -1, -1},
					},
				}
				b, _ := json.Marshal(payload)
				mqttClient.Publish(cmdTopic, 0, false, b)
			}
		}
	}
}

// camHandler fängt den proprietären Bambu-Stream ab, schneidet die 24-Byte Header weg 
// und liefert pures MJPEG an den Browser
func camHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Dem Drucker per MQTT sagen, dass er die Kamera anwerfen soll
	// log.Println("[CAM] Sende webcam_start Kommando...")
	// startCmd := `{"system": {"sequence_id": "1", "command": "webcam_start"}}`
	// mqttClient.Publish(cmdTopic, 0, false, startCmd)

	// Kurz warten, bis der Drucker den Video-Server gestartet hat
	// time.Sleep(3 * time.Second)

	// CORS Headers & MJPEG Content Type setzen
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")

	// 2. TLS-Verbindung zu Port 6000 aufbauen
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
	conf := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", "192.168.178.55:6000", conf)
	if err != nil {
		log.Println("[CAM] TLS Fehler:", err)
		return
	}
	defer conn.Close()

	req := fmt.Sprintf("GET /stream HTTP/1.1\r\nHost: 192.168.178.55:6000\r\nAuthorization: %s\r\n\r\n", auth)
	conn.Write([]byte(req))

	// 3. Den Stream nach JPEGs absuchen (Start: FF D8, Ende: FF D9)
	reader := bufio.NewReader(conn)
	var frame []byte

	log.Println("[CAM] Stream-Proxy läuft!")
	for {
		b, err := reader.ReadByte()
		if err != nil {
			log.Println("[CAM] Stream abgerissen:", err)
			break
		}
		frame = append(frame, b)

		l := len(frame)
		if l >= 2 && frame[l-2] == 0xFF && frame[l-1] == 0xD8 {
			// Wir haben den Start eines neuen JPEGs gefunden -> Puffer zurücksetzen, inkl. FF D8
			frame = []byte{0xFF, 0xD8}
		} else if l >= 2 && frame[l-2] == 0xFF && frame[l-1] == 0xD9 {
			// Ende des JPEGs erreicht -> Frame an den Browser pushen!
			if frame[0] == 0xFF && frame[1] == 0xD8 {
				w.Write([]byte("--frame\r\nContent-Type: image/jpeg\r\n\r\n"))
				w.Write(frame)
				w.Write([]byte("\r\n"))
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
			frame = frame[:0]
		}

		if len(frame) > 1024*1024 { // Schutz gegen RAM-Überlauf
			frame = frame[:0]
		}
	}
}

// filesHandler listet alle .gcode.3mf Dateien auf dem Drucker
func filesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	// curl -k --user "bblp:CODE" ftps://IP:990/
	cmd := exec.Command("curl", "-k", "--user", user+":"+password, ftpsUrl)
	output, err := cmd.Output()
	if err != nil {
		log.Println("[FTP] List Fehler:", err)
		http.Error(w, "Fehler beim Abrufen der Dateiliste", http.StatusInternalServerError)
		return
	}

	var files []string
	lines := strings.Split(string(output), "\n")
	// Einfacher Regex für Dateinamen in der FTP-Liste
	re := regexp.MustCompile(`\d{2}:\d{2}\s+(.*\.gcode\.3mf)$`)

	for _, line := range lines {
		match := re.FindStringSubmatch(line)
		if len(match) > 1 {
			files = append(files, match[1])
		}
	}

	json.NewEncoder(w).Encode(files)
}

// uploadHandler empfängt eine Datei und schiebt sie per FTPS zum Drucker
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != "POST" {
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Keine Datei gefunden", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Datei temporär zwischenspeichern
	tempPath := filepath.Join(os.TempDir(), header.Filename)
	out, err := os.Create(tempPath)
	if err != nil {
		http.Error(w, "Fehler beim Erstellen der Temp-Datei", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempPath)
	io.Copy(out, file)
	out.Close()

	// Per curl zum Drucker schieben
	cmd := exec.Command("curl", "-k", "--user", user+":"+password, "-T", tempPath, ftpsUrl)
	if err := cmd.Run(); err != nil {
		log.Println("[FTP] Upload Fehler:", err)
		http.Error(w, "Upload zum Drucker fehlgeschlagen", http.StatusInternalServerError)
		return
	}

	log.Println("[FTP] Datei erfolgreich hochgeladen:", header.Filename)
	w.Write([]byte("Erfolgreich hochgeladen"))
}

// --- Moonraker API Bridge ---

func moonrakerInfoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Orca prüft, ob der Server antwortet
	resp := map[string]any{
		"result": map[string]any{
			"state": "ready",
			"hostname": "monsterpi",
			"software_version": "v0.1-bambugo",
			"cpu_info": "Raspberry Pi",
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func moonrakerQueryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	latestDataMu.RLock()
	defer latestDataMu.RUnlock()

	// Mappe Bambu Status auf Moonraker Format (sehr vereinfacht)
	tempNozzle := 0.0
	if v, ok := latestData["nozzle_temper"].(float64); ok { tempNozzle = v }
	tempBed := 0.0
	if v, ok := latestData["bed_temper"].(float64); ok { tempBed = v }

	state := "ready"
	if s, ok := latestData["gcode_state"].(string); ok {
		if s == "RUNNING" { state = "printing" }
	}

	resp := map[string]any{
		"result": map[string]any{
			"status": map[string]any{
				"extruder": map[string]any{"temperature": tempNozzle, "target": 0},
				"heater_bed": map[string]any{"temperature": tempBed, "target": 0},
				"print_stats": map[string]any{"state": state},
			},
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func moonrakerUploadHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Datei von Orca entgegennehmen
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Keine Datei", http.StatusBadRequest)
		return
	}
	defer file.Close()

	log.Printf("[Moonraker] Empfange Upload von Orca: %s\n", header.Filename)

	tempPath := filepath.Join(os.TempDir(), header.Filename)
	out, err := os.Create(tempPath)
	if err != nil { return }
	io.Copy(out, file)
	out.Close()

	// 2. Per FTPS zum Drucker schieben
	cmd := exec.Command("curl", "-k", "--user", user+":"+password, "-T", tempPath, ftpsUrl)
	if err := cmd.Run(); err != nil {
		log.Println("[Moonraker] FTP Upload Fehler:", err)
		return
	}
	os.Remove(tempPath)

	// 3. Druck starten via MQTT
	// Wenn es eine .gcode.3mf ist, nutze project_file, sonst gcode_file
	var mqttPayload map[string]any
	if strings.HasSuffix(header.Filename, ".3mf") {
		mqttPayload = map[string]any{
			"print": map[string]any{
				"sequence_id":  nextSequenceID(),
				"command":      "project_file",
				"param":        "Metadata/slice_1.gcode",
				"subtask_name": header.Filename,
				"url":          header.Filename,
				"timelapse":    true,
				"bed_leveling": true,
				"flow_cali":    true,
			},
		}
	} else {
		mqttPayload = map[string]any{
			"print": map[string]any{
				"sequence_id": nextSequenceID(),
				"command":    "gcode_file",
				"param":      header.Filename,
			},
		}
	}

	b, _ := json.Marshal(mqttPayload)
	mqttClient.Publish(cmdTopic, 0, false, b)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"result": "success"})
}
