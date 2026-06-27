package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	headerSize          = 8
	ProtocolVersionV1   = 0x01
	compressionFlag     = 0x01
	compressionLevel    = 3
	compressMinSize     = 128
)

var ErrUnsupportedVersion = errors.New("unsupported protocol version")

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

func Encode(id PacketID, payload []byte, compress bool, seq uint32) []byte {
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
	buf[0] = ProtocolVersionV1
	binary.BigEndian.PutUint16(buf[1:3], uint16(id))
	buf[3] = flags
	binary.BigEndian.PutUint32(buf[4:8], seq)
	copy(buf[headerSize:], data)
	return buf
}

func Decode(packet []byte) (byte, PacketID, []byte, bool, uint32, error) {
	if len(packet) < headerSize {
		return 0, PacketIDInvalid, nil, false, 0, fmt.Errorf("packet too short: %d bytes", len(packet))
	}

	version := packet[0]
	if version != ProtocolVersionV1 {
		return version, PacketIDInvalid, nil, false, 0, ErrUnsupportedVersion
	}

	id := PacketID(binary.BigEndian.Uint16(packet[1:3]))
	flags := packet[3]
	seq := binary.BigEndian.Uint32(packet[4:8])
	compressed := (flags & compressionFlag) != 0
	data := packet[headerSize:]

	if compressed {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return version, PacketIDInvalid, nil, false, 0, fmt.Errorf("gzip reader: %w", err)
		}
		defer r.Close()
		data, err = io.ReadAll(r)
		if err != nil {
			return version, PacketIDInvalid, nil, false, 0, fmt.Errorf("gzip decompress: %w", err)
		}
	}

	return version, id, data, compressed, seq, nil
}
