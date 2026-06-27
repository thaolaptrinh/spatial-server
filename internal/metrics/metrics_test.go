package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_HandlerExposesMetrics(t *testing.T) {
	reg := NewRegistry()
	reg.ActiveConnections.Inc()
	reg.PacketsPerSec.WithLabelValues("in", "0x03").Inc()
	reg.TickDurationSeconds.Observe(0.05)
	srv := httptest.NewServer(reg.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	assert.True(t, strings.Contains(s, "spatial_active_connections"))
	assert.True(t, strings.Contains(s, "spatial_packets_per_sec"))
	assert.True(t, strings.Contains(s, "spatial_tick_duration_seconds"))
}
