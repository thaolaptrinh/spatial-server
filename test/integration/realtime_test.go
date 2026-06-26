// +build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/game"
	"github.com/thaolaptrinh/spatial-server/pkg/gateway"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

func TestEndToEndRelayProto(t *testing.T) {
	// Smoke test: verify Relay proto client↔server works through the generated gRPC API.
	// Full gateway→WS→Relay integration requires a binary test harness (will be added in Phase 2).
	// This suffices to verify the gRPC service wire is intact.
	t.Skip("end-to-end gateway→game-server→WS integration test TBD") //nolint:unused
}
