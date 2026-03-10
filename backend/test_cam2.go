package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	opts := mqtt.NewClientOptions().
		AddBroker("ssl://192.168.178.55:8883").
		SetClientID("test-cam").
		SetUsername("bblp").
		SetPassword("REDACTED_CODE").
		SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}
	c.Publish("device/REDACTED_SERIAL/request", 0, false, `{"system": {"sequence_id": "1", "command": "webcam_start"}}`)
	time.Sleep(3 * time.Second)

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("bblp:REDACTED_CODE"))
	conf := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", "192.168.178.55:6000", conf)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	req := fmt.Sprintf("GET /stream HTTP/1.1\r\nHost: 192.168.178.55:6000\r\nAuthorization: %s\r\n\r\n", auth)
	conn.Write([]byte(req))

	reader := bufio.NewReader(conn)
	for i := 0; i < 1000; i++ {
		b, err := reader.ReadByte()
		if err != nil {
			fmt.Printf("\nERR: %v at %d\n", err, i)
			return
		}
		fmt.Printf("%02x ", b)
	}
}
