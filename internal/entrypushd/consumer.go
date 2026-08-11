package entrypushd

import (
	"context"
	"errors"
	"log/slog"
	"time"

	eventd "github.com/norlis/event-driven/pkg/event"
	"github.com/norlis/event-driven/pkg/eventmux"
	"go.uber.org/fx"
	"nbox/internal/entrypushd/handler"
)

const consumerStopTimeout = 30 * time.Second

// StartConsumer hooks the event subscriber into the fx lifecycle. Each
// received *event.Message goes through h.Handle, wrapped in a panic
// recoverer. Acks every successfully-processed message; Nacks on panic so
// the transport redelivers (redelivery depends on transport).
func StartConsumer(
	lc fx.Lifecycle,
	sub eventmux.Subscription,
	h *handler.Broadcast,
	logger *slog.Logger,
	sd fx.Shutdowner,
) {
	var cancel context.CancelFunc
	done := make(chan struct{})

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			runCtx, c := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is captured in the outer `cancel` var and invoked in OnStop
			cancel = c

			go func() {
				defer close(done)
				err := sub.Start(runCtx, func(msg *eventd.Message) {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("handler panic recovered",
								slog.Any("panic", r),
								slog.String("msg_id", msg.ID()),
							)
							msg.Nack()
							return
						}
					}()
					h.Handle(msg)
					msg.Ack()
				})
				if err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("subscription stopped with error", slog.Any("error", err))
					_ = sd.Shutdown(fx.ExitCode(1))
				}
			}()

			logger.Info("event consumer started")
			return nil
		},
		OnStop: func(_ context.Context) error {
			if cancel != nil {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(consumerStopTimeout):
				logger.Warn("event consumer stop timed out", slog.Duration("after", consumerStopTimeout))
			}
			return nil
		},
	})
}
