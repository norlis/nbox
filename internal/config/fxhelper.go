package config

import (
	"context"

	"go.uber.org/fx"
)

// Activate loads the snapshot synchronously NOW (at provider construction) and
// registers a background refresh ticker. The synchronous load matters: fx runs
// every constructor before any OnStart hook, so loading here guarantees
// Current() is populated before the HTTP/gRPC server's OnStart can accept
// traffic — independent of hook ordering. Fail-closed: returns the load error
// so the provider aborts startup instead of serving without config.
func Activate[T any](lc fx.Lifecycle, snap *Snapshot[T]) error {
	if err := snap.Start(context.Background()); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go snap.Run(ctx)
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
	return nil
}
