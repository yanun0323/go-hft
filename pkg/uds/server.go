package uds

import (
	"errors"
	"main/pkg/exception"
	"net"
	"os"
)

var (
	// ErrNilServer is returned when a nil server receiver is used.
	ErrNilServer = errors.New("uds: nil server")
	// ErrAlreadyListening is returned when Listen is called twice.
	ErrAlreadyListening = errors.New("uds: already listening")
	// ErrNotListening is returned when Accept is called before Listen.
	ErrNotListening = errors.New("uds: not listening")
	// ErrPathNotSocket is returned when the existing path is not a socket.
	ErrPathNotSocket = errors.New("uds: path exists and is not a socket")
)

// Server listens for Unix domain socket connections.
type Server struct {
	addr net.UnixAddr
	ln   *net.UnixListener
}

// NewServer creates a server for the provided socket path.
func NewServer(path string) (*Server, error) {
	if path == "" {
		return nil, exception.ErrEmptyPathUDS
	}
	return &Server{addr: net.UnixAddr{Name: path, Net: unixNetwork}}, nil
}

// Path returns the configured socket path.
func (s *Server) Path() string {
	if s == nil {
		return ""
	}
	return s.addr.Name
}

// Listen starts listening on the configured socket path.
// It removes an existing socket file when present.
func (s *Server) Listen() error {
	if s == nil {
		return ErrNilServer
	}
	if s.addr.Name == "" {
		return exception.ErrEmptyPathUDS
	}
	if s.ln != nil {
		return ErrAlreadyListening
	}
	if err := RemoveIfExists(s.addr.Name); err != nil {
		return err
	}
	ln, err := net.ListenUnix(unixNetwork, &s.addr)
	if err != nil {
		return err
	}
	ln.SetUnlinkOnClose(true)
	s.ln = ln
	return nil
}

// Accept waits for the next incoming connection.
func (s *Server) Accept() (*net.UnixConn, error) {
	if s == nil {
		return nil, ErrNilServer
	}
	if s.ln == nil {
		return nil, ErrNotListening
	}
	return s.ln.AcceptUnix()
}

// Close stops the listener.
func (s *Server) Close() error {
	if s == nil {
		return ErrNilServer
	}
	if s.ln == nil {
		return nil
	}
	err := s.ln.Close()
	s.ln = nil
	return err
}

// RemoveIfExists removes the socket file if it exists.
func RemoveIfExists(path string) error {
	if path == "" {
		return exception.ErrEmptyPathUDS
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return ErrPathNotSocket
	}
	return os.Remove(path)
}
