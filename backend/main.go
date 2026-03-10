package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

const (
	broker   = "ssl://192.168.178.55:8883"
	user     = "bblp"
	password = "b4129081"
	topic    = "device/+/report"
	port     = ":8080"
)

var (
	upgrader = websocket.Upgrader{
		// Erlaube Verbindungen vom Frontend (Vite nutzt meist 5173, wir erlauben alles für lokale Entwicklung)
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
)

func main() {
	// 1. MQTT Client Setup (Verbindung zum Drucker)
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
		} else {
			log.Println("[MQTT] Abonniert auf Topic:", topic)
		}
	}
	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		log.Printf("[MQTT] Verbindung verloren: %v\n", err)
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[MQTT] Verbindungsfehler: %v", token.Error())
	}

	// 2. HTTP/WebSocket Server Setup (Schnittstelle für das React Frontend)
	http.HandleFunc("/ws", wsEndpoint)
	log.Printf("[HTTP] Backend läuft und lauscht auf ws://localhost%s/ws\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("[HTTP] ListenAndServe:", err)
	}
}

// messageHandler empfängt Daten vom Drucker und pusht sie an alle verbundenen WebSockets (Frontend)
func messageHandler(client mqtt.Client, msg mqtt.Message) {
	payload := msg.Payload()

	clientsMu.Lock()
	defer clientsMu.Unlock()

	for conn := range clients {
		err := conn.WriteMessage(websocket.TextMessage, payload)
		if err != nil {
			log.Printf("[WS] Schreibfehler, trenne Client: %v\n", err)
			conn.Close()
			delete(clients, conn)
		}
	}
}

// wsEndpoint wickelt neue WebSocket-Verbindungen vom React-Frontend ab
func wsEndpoint(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("[WS] Upgrade Fehler:", err)
		return
	}
	log.Println("[WS] Neuer Client verbunden")

	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()

	// Endlosschleife, um eingehende Nachrichten vom Frontend abzufangen (z.B. später Steuerbefehle)
	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			log.Println("[WS] Client getrennt")
			clientsMu.Lock()
			delete(clients, ws)
			clientsMu.Unlock()
			break
		}
	}
}
