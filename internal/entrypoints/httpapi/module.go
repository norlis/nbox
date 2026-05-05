package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/entrypoints/api/handlers"
)

var Module = fx.Module("httpapi",
	fx.Provide(presenters.NewPresenters),
	fx.Provide(NewHttpServerMux),
	fx.Invoke(NewHttpApi),
	fx.Provide(handlers.NewUIHandler),

	// fx.Provide(handlers.NewEntryHandler),
	// fx.Provide(handlers.NewBoxHandler),
	// fx.Provide(handlers.NewStaticHandler),
	// fx.Provide(handlers.NewExportHandler),
	// fx.Provide(handlers.NewPrefixConfigHandler),
	// fx.Provide(handlers.NewTrackHandler),
	// fx.Provide(handlers.NewBoxSpecHandler),

	fx.Provide(
		fx.Annotate(handlers.NewEntryHandler, fx.As(new(handlers.Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(handlers.NewBoxHandler, fx.As(new(handlers.Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(handlers.NewStaticHandler, fx.As(new(handlers.Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(handlers.NewExportHandler, fx.As(new(handlers.Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(handlers.NewPrefixConfigHandler, fx.As(new(handlers.Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(handlers.NewTrackHandler, fx.As(new(handlers.Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(handlers.NewBoxSpecHandler, fx.As(new(handlers.Route)), fx.ResultTags(`group:"routes"`)),
	),
)

func NewHttpServerMux(lc fx.Lifecycle, logger *zap.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	listener := net.JoinHostPort(application.Address, application.Port)

	server := &http.Server{
		Addr:              listener,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info(
				"ListenAndServe",
				zap.String("addr", net.JoinHostPort(application.Address, application.Port)),
			)
			go func() {
				if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Error("Error al iniciar servidor HTTP: %v", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Deteniendo servidor HTTP...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := server.Shutdown(shutdownCtx); err != nil {
				logger.Error("Error durante el apagado del servidor HTTP: %v", zap.Error(err))
				return fmt.Errorf("failed to shutdown HTTP server: %w", err)
			}
			logger.Info("Servidor HTTP detenido correctamente.")
			return nil
		},
	})

	return mux
}
