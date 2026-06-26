package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	headerSize       = 3
	compressionFlag  = 0x01
	compressionLevel = 3
	compressMinSize  = 128
)

type PacketID uint16

const (
	PacketIDInvalid        PacketID = 0x00
	PacketIDAuthRequest    PacketID = 0x01
	PacketIDAuthResponse   PacketID = 0x02
	PacketIDPositionUpdate PacketID = 0x03
	PacketIDEntitySpawn    PacketID = 0x04
	PacketIDEntityDespawn  PacketID = 0x05
	PacketIDEntityMove     PacketID = 0x06
	PacketIDEntityAction   PacketID = 0x07
	PacketIDEntityState    PacketID = 0x08
	PacketIDHeartbeat      PacketID = 0xFF
)

func Encode(id PacketID, payload []byte, compress bool) []byte {
	data := payload
	var compressed bool
	if compress && len(payload) > compressMinSize {
		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, compressionLevel)
		if err == nil {
			w.Write(payload) //nolint:errcheck // fallback to uncompressed on error
			w.Close()
			if buf.Len() < len(payload) {
				data = buf.Bytes()
				compressed = true
			}
		}
	}

	var flags byte
	if compressed {
		flags |= compressionFlag
	}

	buf := make([]byte, headerSize+len(data))
	buf[0] = flags
	binary.BigEndian.PutUint16(buf[1:3], uint16(id))
	copy(buf[headerSize:], data)
	return buf
}

func Decode(packet []byte) (PacketID, []byte, bool, error) {
	if len(packet) < headerSize {
		return PacketIDInvalid, nil, false, fmt.Errorf("packet too short: %d bytes", len(packet))
	}

	flags := packet[0]
	id := PacketID(binary.BigEndian.Uint16(packet[1:3]))
	compressed := (flags & compressionFlag) != 0
	data := packet[headerSize:]

	if compressed {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return PacketIDInvalid, nil, false, fmt.Errorf("gzip reader: %w", err)
		}
		defer r.Close()
		data, err = io.ReadAll(r)
		if err != nil {
			return PacketIDInvalid, nil, false, fmt.Errorf("gzip decompress: %w", err)
		}
	}

	return id, data, compressed, nil
}
