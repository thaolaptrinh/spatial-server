package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecode(t *testing.T) {
	payload := []byte("hello")
	packet := Encode(PacketIDPositionUpdate, payload, false, 0)

	version, id, body, compressed, seq, err := Decode(packet)
	assert.NoError(t, err)
	assert.Equal(t, byte(ProtocolVersionV1), version)
	assert.Equal(t, PacketIDPositionUpdate, id)
	assert.Equal(t, payload, body)
	assert.False(t, compressed)
	assert.Equal(t, uint32(0), seq)
}

func TestEncodeDecode_Compressed(t *testing.T) {
	payload := []byte("this is a test payload that should be compressed when large enough to benefit from gzip compression and it is now definitely long enough to exceed the minimum compression threshold so compression will kick in")
	packet := Encode(PacketIDEntitySpawn, payload, true, 0)

	version, id, body, compressed, seq, err := Decode(packet)
	assert.NoError(t, err)
	assert.Equal(t, byte(ProtocolVersionV1), version)
	assert.Equal(t, PacketIDEntitySpawn, id)
	assert.Equal(t, payload, body)
	assert.True(t, compressed)
	assert.Equal(t, uint32(0), seq)
}

func TestDecode_Empty(t *testing.T) {
	_, _, _, _, _, err := Decode(nil)
	assert.Error(t, err)

	_, _, _, _, _, err = Decode([]byte{})
	assert.Error(t, err)
}

func TestDecode_ShortHeader(t *testing.T) {
	_, _, _, _, _, err := Decode([]byte{0x01})
	assert.Error(t, err)
}

func TestEncodeDecode_WithSequence(t *testing.T) {
	payload := []byte("seq-test")
	packet := Encode(PacketIDHeartbeat, payload, false, 42)

	version, id, body, compressed, seq, err := Decode(packet)
	assert.NoError(t, err)
	assert.Equal(t, byte(ProtocolVersionV1), version)
	assert.Equal(t, PacketIDHeartbeat, id)
	assert.Equal(t, payload, body)
	assert.False(t, compressed)
	assert.Equal(t, uint32(42), seq)
}

func TestDecode_VersionMismatch(t *testing.T) {
	payload := []byte("version-test")
	packet := Encode(PacketIDAuthRequest, payload, false, 0)
	packet[0] = 0xFF

	_, _, _, _, _, err := Decode(packet)
	assert.ErrorIs(t, err, ErrUnsupportedVersion)
}

func TestEncodeDecode_LargePayload(t *testing.T) {
	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	packet := Encode(PacketIDEntityState, payload, true, 0)

	version, id, body, compressed, seq, err := Decode(packet)
	assert.NoError(t, err)
	assert.Equal(t, byte(ProtocolVersionV1), version)
	assert.Equal(t, PacketIDEntityState, id)
	assert.Equal(t, payload, body)
	assert.True(t, compressed)
	assert.Equal(t, uint32(0), seq)
}
