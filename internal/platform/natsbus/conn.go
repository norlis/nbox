// Package natsbus provides a shared NATS connection wired into the fx lifecycle.
package natsbus

import (
	"context"
	"fmt"
	"log/slog"

	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"
)

// NewConn dials NATS with infinite auto-reconnect and drains on shutdown.
// name identifies the client in NATS monitoring (e.g. "nbox", "entrypushd").
func NewConn(url, name string, lc fx.Lifecycle, logger *slog.Logger) (*natsgo.Conn, error) {
	if url == "" {
		return nil, fmt.Errorf("natsbus: NATS_URL is required")
	}
	nc, err := natsgo.Connect(url,
		natsgo.Name(name),
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectHandler(func(c *natsgo.Conn) {
			logger.Info("nats reconnected", slog.String("url", c.ConnectedUrl()))
		}),
		natsgo.DisconnectErrHandler(func(_ *natsgo.Conn, e error) {
			if e != nil {
				logger.Warn("nats disconnected", slog.Any("error", e))
			}
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("natsbus: connect %q: %w", url, err)
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error { return nc.Drain() },
	})
	logger.Info("nats connected", slog.String("url", url), slog.String("name", name))
	return nc, nil
}
