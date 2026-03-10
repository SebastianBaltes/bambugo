package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
)

func main() {
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte("bblp:b4129081"))
	conf := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", "192.168.178.55:6000", conf)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	req := fmt.Sprintf("GET /stream HTTP/1.1\r\nHost: 192.168.178.55:6000\r\nAuthorization: %s\r\n\r\n", auth)
	conn.Write([]byte(req))

	reader := bufio.NewReader(conn)
	b := make([]byte, 100)
	n, _ := reader.Read(b)
	fmt.Printf("READ: %x\n", b[:n])
}
