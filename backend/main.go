package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
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
	cmdTopic = "device/22E8BJ5C1401719/request" // Dein Drucker
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
	
	log.Printf("[HTTP] Backend läuft auf %s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("[HTTP] ListenAndServe:", err)
	}
}

func messageHandler(client mqtt.Client, msg mqtt.Message) {
	payload := msg.Payload()
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
