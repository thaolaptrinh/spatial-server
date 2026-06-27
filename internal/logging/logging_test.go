package logging

import (
	"context"
	"testing"
)

func TestNewDefault(t *testing.T) {
	logger := NewDefault("test-service")
	if logger == nil {
		t.Fatal("NewDefault returned nil")
	}
}

func TestContextRoundTrip(t *testing.T) {
	logger := NewDefault("test")
	ctx := WithLogger(context.Background(), logger)
	got := FromContext(ctx)
	if got != logger {
		t.Error("FromContext did not return the same logger")
	}
}

func TestFromContextDefault(t *testing.T) {
	got := FromContext(context.Background())
	if got == nil {
		t.Fatal("FromContext returned nil")
	}
}

func TestStandardKeysAreSet(t *testing.T) {
	keys := []string{ServiceKey, TraceIDKey, RequestIDKey, SessionIDKey, ZoneIDKey, EntityIDKey, RuntimeIDKey}
	for _, k := range keys {
		if k == "" {
			t.Error("standard key is empty")
		}
	}
}
