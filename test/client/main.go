package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"time"
)

const _sockPath = "/tmp/unix.sock"

func main() {
	conn, err := net.Dial("unix", _sockPath)
	if err != nil {
		panic(err)
	}

	b := bytes.Buffer{}
	for range 100 {
		b.WriteString("0123456789")
	}

	start := time.Now()
	if _, err := conn.Write(b.Bytes()); err != nil {
		log.Print(err)
		return
	}
	var buf = make([]byte, 1024)
	if _, err := conn.Read(buf); err != nil {
		panic(err)
	}
	d := time.Since(start)

	fmt.Println("client recv:", string(buf), "cost:", d)
}
