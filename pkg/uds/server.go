package uds

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

type Server struct {
	Path string
}

func (s Server) Run() {
	os.Remove(_sockPath)
	l, err := net.Listen("unix", s.Path)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
}

const _sockPath = "/tmp/unix.sock"

func main() {
	os.Remove(_sockPath)
	l, err := net.Listen("unix", _sockPath)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		fmt.Println("local addr: ", conn.LocalAddr().String())
		if err != nil {
			log.Fatal(err)
		}
		go func(c net.Conn) {
			for {
				buf := make([]byte, 1024)
				_, err := c.Read(buf)
				if err != nil && err != io.EOF {
					c.Close()
					break
				}
				if err == io.EOF {
					break
				}
				println("recv: ", string(buf))
				println()
			}
			c.Close()
		}(conn)

	}
}
