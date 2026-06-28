//go:build validation

package recovery

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type FixedDelayCondition struct {
	name    string
	delay   time.Duration
	started time.Time
}

func FixedDelay(name string, delay time.Duration) *FixedDelayCondition {
	return &FixedDelayCondition{name: name, delay: delay, started: time.Now()}
}

func (c *FixedDelayCondition) Name() string { return c.name }
func (c *FixedDelayCondition) Met(ctx context.Context, infra validation.Infrastructure) (bool, error) {
	return time.Since(c.started) >= c.delay, nil
}

type HealthyEndpointCondition struct {
	resource validation.ResourceID
}

func HealthyEndpoint(resource validation.ResourceID) *HealthyEndpointCondition {
	return &HealthyEndpointCondition{resource: resource}
}

func (c *HealthyEndpointCondition) Name() string { return fmt.Sprintf("healthy(%s)", c.resource) }
func (c *HealthyEndpointCondition) Met(ctx context.Context, infra validation.Infrastructure) (bool, error) {
	addr, err := infra.Endpoint(c.resource)
	if err != nil {
		return false, nil
	}
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}
