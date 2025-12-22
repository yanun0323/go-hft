package recorder

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"

	"main/internal/schema"
)

const (
	recordVersion      uint16 = 1
	recordHeaderSize          = 56
	recordChecksumSize        = 4
)

var (
	recordMagic = [4]byte{'W', 'A', 'L', '1'}
	crcTable   = crc32.MakeTable(crc32.Castagnoli)
)

var (
	ErrInvalidMagic            = errors.New("wal invalid magic")
	ErrUnsupportedRecordVer    = errors.New("wal unsupported record version")
	ErrInvalidRecordHeaderSize = errors.New("wal invalid header size")
)

func encodeHeader(dst []byte, header schema.EventHeader, payloadLen int) {
	_ = dst[recordHeaderSize-1]
	copy(dst[0:4], recordMagic[:])
	binary.LittleEndian.PutUint16(dst[4:6], recordVersion)
	binary.LittleEndian.PutUint16(dst[6:8], uint16(recordHeaderSize))
	binary.LittleEndian.PutUint16(dst[8:10], uint16(header.Type))
	binary.LittleEndian.PutUint16(dst[10:12], header.Version)
	binary.LittleEndian.PutUint16(dst[12:14], header.Source)
	binary.LittleEndian.PutUint16(dst[14:16], header.Flags)
	binary.LittleEndian.PutUint32(dst[16:20], uint32(payloadLen))
	binary.LittleEndian.PutUint64(dst[20:28], header.Seq)
	binary.LittleEndian.PutUint64(dst[28:36], uint64(header.TsEvent))
	binary.LittleEndian.PutUint64(dst[36:44], uint64(header.TsRecv))
	binary.LittleEndian.PutUint64(dst[44:52], header.TraceID)
	binary.LittleEndian.PutUint32(dst[52:56], 0)
}

func checksum(header []byte, payload []byte) uint32 {
	crc := crc32.Update(0, crcTable, header)
	return crc32.Update(crc, crcTable, payload)
}

func decodeRecordHeader(src []byte) (schema.EventHeader, uint32, error) {
	if len(src) < recordHeaderSize {
		return schema.EventHeader{}, 0, ErrInvalidRecordHeaderSize
	}
	if !bytes.Equal(src[0:4], recordMagic[:]) {
		return schema.EventHeader{}, 0, ErrInvalidMagic
	}
	if ver := binary.LittleEndian.Uint16(src[4:6]); ver != recordVersion {
		return schema.EventHeader{}, 0, ErrUnsupportedRecordVer
	}
	if headerSize := binary.LittleEndian.Uint16(src[6:8]); headerSize != recordHeaderSize {
		return schema.EventHeader{}, 0, ErrInvalidRecordHeaderSize
	}
	payloadLen := binary.LittleEndian.Uint32(src[16:20])
	h := schema.EventHeader{
		Type:    schema.EventType(binary.LittleEndian.Uint16(src[8:10])),
		Version: binary.LittleEndian.Uint16(src[10:12]),
		Source:  binary.LittleEndian.Uint16(src[12:14]),
		Flags:   binary.LittleEndian.Uint16(src[14:16]),
		Seq:     binary.LittleEndian.Uint64(src[20:28]),
		TsEvent: int64(binary.LittleEndian.Uint64(src[28:36])),
		TsRecv:  int64(binary.LittleEndian.Uint64(src[36:44])),
		TraceID: binary.LittleEndian.Uint64(src[44:52]),
	}
	return h, payloadLen, nil
}
