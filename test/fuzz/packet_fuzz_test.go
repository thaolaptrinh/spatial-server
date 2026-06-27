package fuzz

import (
	"testing"

	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
)

func FuzzPacketDecode(f *testing.F) {
	f.Add([]byte{0x01, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01, 0x42})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _, _, _, err := protocol.Decode(data)
		_ = err
	})
}
