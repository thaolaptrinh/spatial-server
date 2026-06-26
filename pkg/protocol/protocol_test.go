package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecode(t *testing.T) {
	payload := []byte("hello")
	packet := Encode(PacketIDPositionUpdate, payload, false)

	id, body, compressed, err := Decode(packet)
	assert.NoError(t, err)
	assert.Equal(t, PacketIDPositionUpdate, id)
	assert.Equal(t, payload, body)
	assert.False(t, compressed)
}

func TestEncodeDecode_Compressed(t *testing.T) {
	payload := []byte("this is a test payload that should be compressed when large enough to benefit from gzip compression and it is now definitely long enough to exceed the minimum compression threshold so compression will kick in")
	packet := Encode(PacketIDEntitySpawn, payload, true)

	id, body, compressed, err := Decode(packet)
	assert.NoError(t, err)
	assert.Equal(t, PacketIDEntitySpawn, id)
	assert.Equal(t, payload, body)
	assert.True(t, compressed)
}

func TestDecode_Empty(t *testing.T) {
	_, _, _, err := Decode(nil)
	assert.Error(t, err)

	_, _, _, err = Decode([]byte{})
	assert.Error(t, err)
}

func TestDecode_ShortHeader(t *testing.T) {
	_, _, _, err := Decode([]byte{0x01})
	assert.Error(t, err)
}

func TestEncodeDecode_LargePayload(t *testing.T) {
	payload := make([]byte, 1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	packet := Encode(PacketIDEntityState, payload, true)

	id, body, compressed, err := Decode(packet)
	assert.NoError(t, err)
	assert.Equal(t, PacketIDEntityState, id)
	assert.Equal(t, payload, body)
	assert.True(t, compressed)
}
