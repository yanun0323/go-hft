package recorder

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"

	"main/internal/schema"
)

var ErrChecksumMismatch = errors.New("wal checksum mismatch")

// ReaderOptions controls record decoding.
type ReaderOptions struct {
	DisableChecksum bool
	MaxPayloadSize  int
}

// Reader decodes WAL records sequentially.
type Reader struct {
	r         *bufio.Reader
	opts      ReaderOptions
	headerBuf []byte
	payload   []byte
}

// NewReader wraps an io.Reader with WAL decoding.
func NewReader(r io.Reader, opts ReaderOptions) *Reader {
	return &Reader{
		r:         bufio.NewReader(r),
		opts:      opts,
		headerBuf: make([]byte, recordHeaderSize),
	}
}

// Next returns the next record header and payload.
// The payload is only valid until the next call to Next.
func (r *Reader) Next() (schema.EventHeader, []byte, error) {
	var header schema.EventHeader

	n, err := io.ReadFull(r.r, r.headerBuf)
	if err != nil {
		if err == io.EOF && n == 0 {
			return header, nil, io.EOF
		}
		return header, nil, err
	}

	header, payloadLen, err := decodeRecordHeader(r.headerBuf)
	if err != nil {
		return header, nil, err
	}
	if r.opts.MaxPayloadSize > 0 && payloadLen > uint32(r.opts.MaxPayloadSize) {
		return header, nil, ErrPayloadTooLarge
	}
	if uint64(payloadLen) > maxPayloadLen {
		return header, nil, ErrPayloadTooLarge
	}

	if payloadLen > 0 {
		if cap(r.payload) < int(payloadLen) {
			r.payload = make([]byte, payloadLen)
		}
		r.payload = r.payload[:payloadLen]
		if _, err := io.ReadFull(r.r, r.payload); err != nil {
			return header, nil, err
		}
	} else {
		r.payload = r.payload[:0]
	}

	var checksumBuf [recordChecksumSize]byte
	if _, err := io.ReadFull(r.r, checksumBuf[:]); err != nil {
		return header, nil, err
	}

	if !r.opts.DisableChecksum {
		expected := binary.LittleEndian.Uint32(checksumBuf[:])
		sum := checksum(r.headerBuf, r.payload)
		if sum != expected {
			return header, nil, ErrChecksumMismatch
		}
	}

	return header, r.payload, nil
}
