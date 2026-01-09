package uds

import (
	"main/pkg/exception"
	"net"
)

const unixNetwork = "unix"

// Client dials Unix domain sockets using a precomputed address.
type Client struct {
	addr net.UnixAddr
}

// NewClient creates a client for the provided socket path.
func NewClient(path string) (*Client, error) {
	if path == "" {
		return nil, exception.ErrEmptyPathUDS
	}
	return &Client{addr: net.UnixAddr{Name: path, Net: unixNetwork}}, nil
}

// Path returns the configured socket path.
func (c *Client) Path() string {
	if c == nil {
		return ""
	}
	return c.addr.Name
}

// Dial opens a Unix domain socket connection.
func (c *Client) Dial() (*net.UnixConn, error) {
	if c == nil {
		return nil, exception.ErrNilClientUDS
	}
	if c.addr.Name == "" {
		return nil, exception.ErrEmptyPathUDS
	}
	return net.DialUnix(unixNetwork, nil, &c.addr)
}
