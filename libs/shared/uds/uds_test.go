package uds

import (
	"main/libs/shared/exception"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewClientEmptyPath(t *testing.T) {
	if _, err := NewClient(""); err != exception.ErrEmptyPathUDS {
		t.Fatalf("expected ErrEmptyPath, got %v", err)
	}
}

func TestNewServerEmptyPath(t *testing.T) {
	if _, err := NewServer(""); err != exception.ErrEmptyPathUDS {
		t.Fatalf("expected ErrEmptyPath, got %v", err)
	}
}

func TestRemoveIfExistsRejectsNonSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-socket")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := RemoveIfExists(path); err != ErrPathNotSocket {
		t.Fatalf("expected ErrPathNotSocket, got %v", err)
	}
}

func TestServerDialAccept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uds.sock")

	server, err := NewServer(path)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer server.Close()

	acceptCh := make(chan *net.UnixConn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := server.Accept()
		if err != nil {
			errCh <- err
			return
		}
		acceptCh <- conn
	}()

	client, err := NewClient(path)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	conn, err := client.Dial()
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	select {
	case err := <-errCh:
		t.Fatalf("Accept: %v", err)
	case serverConn := <-acceptCh:
		serverConn.Close()
	case <-timer.C:
		t.Fatal("timeout waiting for accept")
	}

	if err := server.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected socket path removed, got %v", err)
	}
}

func BenchmarkUDSTransfer64B(b *testing.B) {
	benchmarkUDSTransfer(b, 64)
}

func BenchmarkUDSTransfer4KB(b *testing.B) {
	benchmarkUDSTransfer(b, 4*1024)
}

func benchmarkUDSTransfer(b *testing.B, payloadSize int) {
	b.Helper()
	if payloadSize <= 0 {
		b.Fatalf("payload size must be positive")
	}

	dir := b.TempDir()
	path := filepath.Join(dir, "uds.sock")

	server, err := NewServer(path)
	if err != nil {
		b.Fatalf("NewServer: %v", err)
	}
	if err := server.Listen(); err != nil {
		b.Fatalf("Listen: %v", err)
	}
	defer server.Close()

	readyCh := make(chan struct{})
	serverErrCh := make(chan error, 1)
	stopCh := make(chan struct{})
	totalBytes := int64(payloadSize) * int64(b.N)

	go func() {
		conn, err := server.Accept()
		if err != nil {
			serverErrCh <- err
			return
		}
		defer conn.Close()
		close(readyCh)

		buf := make([]byte, payloadSize)
		var readBytes int64
		for readBytes < totalBytes {
			select {
			case <-stopCh:
				serverErrCh <- nil
				return
			default:
			}

			n, err := conn.Read(buf)
			if err != nil {
				select {
				case <-stopCh:
					serverErrCh <- nil
				default:
					serverErrCh <- err
				}
				return
			}
			readBytes += int64(n)
		}
		serverErrCh <- nil
	}()

	client, err := NewClient(path)
	if err != nil {
		b.Fatalf("NewClient: %v", err)
	}
	conn, err := client.Dial()
	if err != nil {
		b.Fatalf("Dial: %v", err)
	}
	defer func() {
		close(stopCh)
		_ = conn.Close()
	}()

	<-readyCh

	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i)
	}

	b.ReportAllocs()
	b.SetBytes(int64(payloadSize))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := writeFull(conn, payload); err != nil {
			b.Fatalf("write: %v", err)
		}
	}

	b.StopTimer()

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case err := <-serverErrCh:
		if err != nil {
			b.Fatalf("server read: %v", err)
		}
	case <-timer.C:
		b.Fatal("timeout waiting for server read")
	}
}

func writeFull(conn *net.UnixConn, buf []byte) error {
	for len(buf) > 0 {
		n, err := conn.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}
