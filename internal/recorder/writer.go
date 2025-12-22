package recorder

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"main/internal/schema"
)

var (
	ErrQueueFull       = errors.New("wal queue full")
	ErrClosed          = errors.New("wal writer closed")
	ErrNotStarted      = errors.New("wal writer not started")
	ErrAlreadyStarted  = errors.New("wal writer already started")
	ErrPayloadTooLarge = errors.New("wal payload too large")
)

const maxPayloadLen = uint64(^uint32(0))

// Writer appends events to WAL segments from a buffered queue.
type Writer struct {
	cfg Config
	ch  chan recordRequest
	wg  sync.WaitGroup
	err atomic.Value

	started uint32
	closed  uint32
}

// NewWriter creates a WAL writer and ensures the target directory exists.
func NewWriter(cfg Config) (*Writer, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, err
	}
	w := &Writer{
		cfg: cfg,
		ch:  make(chan recordRequest, cfg.QueueSize),
	}
	return w, nil
}

// Start runs the writer loop in a new goroutine.
func (w *Writer) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32(&w.started, 0, 1) {
		return ErrAlreadyStarted
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.run(ctx)
	}()
	return nil
}

// Close stops the writer and flushes any buffered data.
func (w *Writer) Close() error {
	if atomic.CompareAndSwapUint32(&w.closed, 0, 1) {
		close(w.ch)
	}
	w.wg.Wait()
	return w.Err()
}

// Err returns the first error observed by the writer, if any.
func (w *Writer) Err() error {
	if v := w.err.Load(); v != nil {
		return v.(error)
	}
	return nil
}

// TryAppend enqueues an event without blocking.
func (w *Writer) TryAppend(header schema.EventHeader, payload []byte) error {
	if atomic.LoadUint32(&w.closed) != 0 {
		return ErrClosed
	}
	if atomic.LoadUint32(&w.started) == 0 {
		return ErrNotStarted
	}
	if err := w.Err(); err != nil {
		return err
	}
	if uint64(len(payload)) > maxPayloadLen {
		return ErrPayloadTooLarge
	}
	if header.Version == 0 {
		header.Version = schema.SchemaVersion
	}
	if w.cfg.CopyPayload && len(payload) > 0 {
		cp := make([]byte, len(payload))
		copy(cp, payload)
		payload = cp
	}

	req := recordRequest{header: header, payload: payload}
	select {
	case w.ch <- req:
		return nil
	default:
		return ErrQueueFull
	}
}

func (w *Writer) run(ctx context.Context) {
	var (
		seg         *segmentWriter
		segID       uint64
		headerBuf   = make([]byte, recordHeaderSize)
		checksumBuf [4]byte
		flushC      <-chan time.Time
		syncC       <-chan time.Time
		flushTicker *time.Ticker
		syncTicker  *time.Ticker
	)

	if w.cfg.FlushInterval > 0 {
		flushTicker = time.NewTicker(w.cfg.FlushInterval)
		flushC = flushTicker.C
	}
	if w.cfg.SyncInterval > 0 {
		syncTicker = time.NewTicker(w.cfg.SyncInterval)
		syncC = syncTicker.C
	}

	defer func() {
		if flushTicker != nil {
			flushTicker.Stop()
		}
		if syncTicker != nil {
			syncTicker.Stop()
		}
		if err := w.closeSegment(seg); err != nil && w.Err() == nil {
			w.setErr(err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			w.drainNonBlocking(&seg, &segID, headerBuf, &checksumBuf)
			return
		case req, ok := <-w.ch:
			if !ok {
				return
			}
			if err := w.writeRecord(&seg, &segID, headerBuf, &checksumBuf, req); err != nil {
				w.setErr(err)
				return
			}
		case <-flushC:
			if err := w.flushSegment(seg); err != nil {
				w.setErr(err)
				return
			}
		case <-syncC:
			if err := w.syncSegment(seg); err != nil {
				w.setErr(err)
				return
			}
		}
	}
}

func (w *Writer) drainNonBlocking(seg **segmentWriter, segID *uint64, headerBuf []byte, checksumBuf *[4]byte) {
	for {
		select {
		case req, ok := <-w.ch:
			if !ok {
				return
			}
			if err := w.writeRecord(seg, segID, headerBuf, checksumBuf, req); err != nil {
				w.setErr(err)
				return
			}
		default:
			return
		}
	}
}

func (w *Writer) writeRecord(seg **segmentWriter, segID *uint64, headerBuf []byte, checksumBuf *[4]byte, req recordRequest) error {
	if uint64(len(req.payload)) > maxPayloadLen {
		return ErrPayloadTooLarge
	}

	now := time.Now().UTC()
	recordSize := int64(recordHeaderSize + len(req.payload) + recordChecksumSize)
	if w.shouldRotate(*seg, now, recordSize) {
		if err := w.closeSegment(*seg); err != nil {
			return err
		}
		opened, err := w.openSegment(segID, now)
		if err != nil {
			return err
		}
		*seg = opened
	}

	encodeHeader(headerBuf, req.header, len(req.payload))
	sum := checksum(headerBuf, req.payload)
	binary.LittleEndian.PutUint32(checksumBuf[:], sum)

	if _, err := (*seg).buf.Write(headerBuf); err != nil {
		return err
	}
	if len(req.payload) > 0 {
		if _, err := (*seg).buf.Write(req.payload); err != nil {
			return err
		}
	}
	if _, err := (*seg).buf.Write(checksumBuf[:]); err != nil {
		return err
	}

	(*seg).size += recordSize
	return nil
}

func (w *Writer) shouldRotate(seg *segmentWriter, now time.Time, nextSize int64) bool {
	if seg == nil {
		return true
	}
	if w.cfg.SegmentMaxBytes > 0 && seg.size+nextSize > w.cfg.SegmentMaxBytes {
		return true
	}
	if w.cfg.SegmentMaxDuration > 0 && now.Sub(seg.openedAt) >= w.cfg.SegmentMaxDuration {
		return true
	}
	return false
}

func (w *Writer) flushSegment(seg *segmentWriter) error {
	if seg == nil {
		return nil
	}
	return seg.buf.Flush()
}

func (w *Writer) syncSegment(seg *segmentWriter) error {
	if seg == nil {
		return nil
	}
	if err := seg.buf.Flush(); err != nil {
		return err
	}
	return seg.file.Sync()
}

func (w *Writer) closeSegment(seg *segmentWriter) error {
	if seg == nil {
		return nil
	}
	if err := seg.buf.Flush(); err != nil {
		_ = seg.file.Close()
		return err
	}
	if err := seg.file.Sync(); err != nil {
		_ = seg.file.Close()
		return err
	}
	return seg.file.Close()
}

func (w *Writer) openSegment(segID *uint64, now time.Time) (*segmentWriter, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ts := now.Format("20060102-150405")
	for {
		*segID = *segID + 1
		name := fmt.Sprintf("%s-%s-%06d.wal", w.cfg.FilePrefix, ts, *segID)
		path := filepath.Join(w.cfg.Dir, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, err
		}
		return &segmentWriter{
			file:     file,
			buf:      bufio.NewWriterSize(file, w.cfg.BufferSize),
			openedAt: now,
		}, nil
	}
}

func (w *Writer) setErr(err error) {
	if err == nil {
		return
	}
	if w.err.Load() != nil {
		return
	}
	w.err.Store(err)
}

type recordRequest struct {
	header  schema.EventHeader
	payload []byte
}

type segmentWriter struct {
	file     *os.File
	buf      *bufio.Writer
	size     int64
	openedAt time.Time
}
