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

type Config struct {
	PrinterIP         string `json:"printer_ip"`
	PrinterSerial     string `json:"printer_serial"`
	PrinterAccessCode string `json:"printer_access_code"`
	BackendPort       string `json:"backend_port"`
}

var (
	config      Config
	configMu    sync.RWMutex
	configPath  = "config.json"
	
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

func loadConfig() error {
	file, err := os.ReadFile(configPath)
	if err != nil { return err }
	configMu.Lock()
	defer configMu.Unlock()
	return json.Unmarshal(file, &config)
}

func saveConfig(c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil { return err }
	return os.WriteFile(configPath, data, 0644)
}

func main() {
	// Debug-Log in Datei umleiten
	logFile, _ := os.OpenFile("debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	if err := loadConfig(); err != nil {
		log.Printf("Warnung: Konnte %s nicht laden: %v\n", configPath, err)
		config = Config{
			PrinterIP: "192.168.178.55",
			PrinterSerial: "22E8BJ5C1401719",
			PrinterAccessCode: "b4129081",
			BackendPort: ":8080",
		}
	}

	initMQTT()

	http.HandleFunc("/ws", wsEndpoint)
	http.HandleFunc("/stream", camHandler)
	http.HandleFunc("/files", filesHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/api/config", configHandler)
	
	http.HandleFunc("/", logRequest(rootHandler))
	http.HandleFunc("/api/version", logRequest(octoVersionHandler))
	http.HandleFunc("/printer/info", logRequest(moonrakerInfoHandler))
	http.HandleFunc("/server/files/upload", logRequest(moonrakerUploadHandler))
	http.HandleFunc("/printer/objects/query", logRequest(moonrakerQueryHandler))
	
	log.Printf("[HTTP] Backend läuft auf %s\n", config.BackendPort)
	if err := http.ListenAndServe(config.BackendPort, nil); err != nil {
		log.Fatal("[HTTP] ListenAndServe:", err)
	}
}

func initMQTT() {
	if mqttClient != nil && mqttClient.IsConnected() {
		mqttClient.Disconnect(250)
	}
	configMu.RLock()
	c := config
	configMu.RUnlock()
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("ssl://%s:8883", c.PrinterIP)).
		SetClientID("bambugo-backend").
		SetUsername("bblp").
		SetPassword(c.PrinterAccessCode).
		SetTLSConfig(tlsConfig)
	opts.OnConnect = func(cl mqtt.Client) {
		log.Println("[MQTT] Verbunden!")
		topic := fmt.Sprintf("device/%s/report", c.PrinterSerial)
		cl.Subscribe(topic, 0, messageHandler)
	}
	mqttClient = mqtt.NewClient(opts)
	mqttClient.Connect()
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == "GET" {
		configMu.RLock()
		defer configMu.RUnlock()
		json.NewEncoder(w).Encode(config)
	} else if r.Method == "POST" {
		var newConfig Config
		json.NewDecoder(r.Body).Decode(&newConfig)
		saveConfig(newConfig)
		configMu.Lock()
		config = newConfig
		configMu.Unlock()
		go initMQTT()
		w.Write([]byte("OK"))
	}
}

func logRequest(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HTTP] %s %s\n", r.Method, r.URL.Path)
		h(w, r)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) { w.Write([]byte("BambuGo Backend")) }
func octoVersionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"api": "0.1", "server": "1.3.10", "text": "OctoPrint (BambuGo Bridge)"})
}

func messageHandler(client mqtt.Client, msg mqtt.Message) {
	payload := msg.Payload()
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err == nil {
		if printObj, ok := data["print"].(map[string]any); ok {
			latestDataMu.Lock()
			if latestData == nil { latestData = make(map[string]any) }
			for k, v := range printObj { latestData[k] = v }
			latestDataMu.Unlock()
		}
	}
	clientsMu.Lock()
	defer clientsMu.Unlock()
	for conn := range clients {
		conn.WriteMessage(websocket.TextMessage, payload)
	}
}

func nextSequenceID() int { return int(atomic.AddUint64(&seqCounter, 1)) }

func wsEndpoint(w http.ResponseWriter, r *http.Request) {
	ws, _ := upgrader.Upgrade(w, r, nil)
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
		if command == "light_on" {
			sendMQTT(map[string]any{"system": map[string]any{"sequence_id": strconv.Itoa(nextSequenceID()), "command": "ledctrl", "led_node": "chamber_light", "led_mode": "on", "led_on_time": 0, "led_off_time": 0, "loop_times": 0, "interval_time": 0}})
		} else if command == "light_off" {
			sendMQTT(map[string]any{"system": map[string]any{"sequence_id": strconv.Itoa(nextSequenceID()), "command": "ledctrl", "led_node": "chamber_light", "led_mode": "off", "led_on_time": 0, "led_off_time": 0, "loop_times": 0, "interval_time": 0}})
		} else if command == "print_pause" {
			sendMQTT(map[string]any{"print": map[string]any{"sequence_id": nextSequenceID(), "command": "pause"}})
		} else if command == "print_resume" {
			sendMQTT(map[string]any{"print": map[string]any{"sequence_id": nextSequenceID(), "command": "resume"}})
		} else if command == "print_stop" {
			sendMQTT(map[string]any{"print": map[string]any{"sequence_id": nextSequenceID(), "command": "stop"}})
		} else if strings.HasPrefix(command, "print_file:") {
			filename := strings.TrimPrefix(command, "print_file:")
			log.Printf("[CMD] Starte Shotgun-Diagnose für: %s\n", filename)
			
			// Wir feuern 4 verschiedene Varianten ab. Eine davon MUSS klappen.
			// Variante 1: Standard SD
			sendMQTT(map[string]any{"print": map[string]any{"sequence_id": 1001, "command": "project_file", "param": "Metadata/slice_1.gcode", "url": "file:///sdcard/" + filename}})
			// Variante 2: Standard USB
			sendMQTT(map[string]any{"print": map[string]any{"sequence_id": 1002, "command": "project_file", "param": "Metadata/slice_1.gcode", "url": "file:///usb/" + filename}})
			// Variante 3: GCode-Style SD
			sendMQTT(map[string]any{"print": map[string]any{"sequence_id": 1003, "command": "gcode_file", "param": "/sdcard/" + filename}})
			// Variante 4: GCode-Style USB
			sendMQTT(map[string]any{"print": map[string]any{"sequence_id": 1004, "command": "gcode_file", "param": "/usb/" + filename}})
		}
	}
}

func sendMQTT(payload any) {
	b, _ := json.Marshal(payload)
	configMu.RLock()
	topic := fmt.Sprintf("device/%s/request", config.PrinterSerial)
	configMu.RUnlock()
	log.Printf("[MQTT] Sende: %s\n", string(b))
	mqttClient.Publish(topic, 0, false, b)
}

func camHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	configMu.RLock()
	c := config
	configMu.RUnlock()
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("bblp:"+c.PrinterAccessCode))
	conf := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", c.PrinterIP+":6000", conf)
	if err != nil { return }
	defer conn.Close()
	req := fmt.Sprintf("GET /stream HTTP/1.1\r\nHost: %s:6000\r\nAuthorization: %s\r\n\r\n", c.PrinterIP, auth)
	conn.Write([]byte(req))
	reader := bufio.NewReader(conn)
	var frame []byte
	for {
		b, err := reader.ReadByte()
		if err != nil { break }
		frame = append(frame, b)
		l := len(frame)
		if l >= 2 && frame[l-2] == 0xFF && frame[l-1] == 0xD8 {
			frame = []byte{0xFF, 0xD8}
		} else if l >= 2 && frame[l-2] == 0xFF && frame[l-1] == 0xD9 {
			if frame[0] == 0xFF && frame[1] == 0xD8 {
				w.Write([]byte("--frame\r\nContent-Type: image/jpeg\r\n\r\n"))
				w.Write(frame)
				w.Write([]byte("\r\n"))
				if flusher, ok := w.(http.Flusher); ok { flusher.Flush() }
			}
			frame = frame[:0]
		}
		if len(frame) > 1024*1024 { frame = frame[:0] }
	}
}

func filesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	configMu.RLock()
	c := config
	configMu.RUnlock()
	cmd := exec.Command("curl", "-k", "--user", "bblp:"+c.PrinterAccessCode, "ftps://"+c.PrinterIP+":990/")
	output, err := cmd.Output()
	if err != nil { return }
	var files []string
	lines := strings.Split(string(output), "\n")
	re := regexp.MustCompile(`\d{2}:\d{2}\s+(.*\.gcode\.3mf)$`)
	for _, line := range lines {
		match := re.FindStringSubmatch(line)
		if len(match) > 1 { files = append(files, match[1]) }
	}
	json.NewEncoder(w).Encode(files)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != "POST" { return }
	file, header, err := r.FormFile("file")
	if err != nil { return }
	defer file.Close()
	tempPath := filepath.Join(os.TempDir(), header.Filename)
	out, _ := os.Create(tempPath)
	io.Copy(out, file)
	out.Close()
	configMu.RLock()
	c := config
	configMu.RUnlock()
	exec.Command("curl", "-k", "--user", "bblp:"+c.PrinterAccessCode, "-T", tempPath, "ftps://"+c.PrinterIP+":990/").Run()
	os.Remove(tempPath)
	w.Write([]byte("OK"))
}

func moonrakerInfoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"state": "ready", "hostname": "monsterpi", "software_version": "v0.1-bambugo", "cpu_info": "Raspberry Pi"}})
}

func moonrakerQueryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	latestDataMu.RLock()
	defer latestDataMu.RUnlock()
	tempNozzle := 0.0
	if v, ok := latestData["nozzle_temper"].(float64); ok { tempNozzle = v }
	tempBed := 0.0
	if v, ok := latestData["bed_temper"].(float64); ok { tempBed = v }
	state := "ready"
	if s, ok := latestData["gcode_state"].(string); ok {
		if s == "RUNNING" { state = "printing" }
	}
	json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"status": map[string]any{"extruder": map[string]any{"temperature": tempNozzle, "target": 0}, "heater_bed": map[string]any{"temperature": tempBed, "target": 0}, "print_stats": map[string]any{"state": state}}}})
}

func moonrakerUploadHandler(w http.ResponseWriter, r *http.Request) {
	file, header, _ := r.FormFile("file")
	defer file.Close()
	tempPath := filepath.Join(os.TempDir(), header.Filename)
	out, _ := os.Create(tempPath)
	io.Copy(out, file)
	out.Close()
	configMu.RLock()
	c := config
	configMu.RUnlock()
	exec.Command("curl", "-k", "--user", "bblp:"+c.PrinterAccessCode, "-T", tempPath, "ftps://"+c.PrinterIP+":990/").Run()
	os.Remove(tempPath)
	payload := map[string]any{"print": map[string]any{"sequence_id": nextSequenceID(), "command": "gcode_file", "param": header.Filename}}
	b, _ := json.Marshal(payload)
	topic := fmt.Sprintf("device/%s/request", c.PrinterSerial)
	mqttClient.Publish(topic, 0, false, b)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"result": "success"})
}
