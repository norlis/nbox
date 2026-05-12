package transporthttp

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
	auth "nbox/internal/auth"
	"nbox/internal/box"
	"nbox/internal/entry"
	"nbox/internal/export"
	"nbox/internal/prefix"
	"nbox/internal/tracking"
	authmw "nbox/internal/transport/http/middleware"
)

var Module = fx.Module("transport.http",
	fx.Provide(presenters.NewPresenters),
	fx.Provide(NewServeMux),
	fx.Invoke(NewServer),
	fx.Provide(NewUIHandler),

	// Authn middleware — config-driven, lives at the transport layer.
	fx.Provide(func(config *application.Config, render presenters.Presenters, logger *zap.Logger, repo auth.Store) *authmw.Authn {
		return authmw.NewAuthn(config.CredentialsLoader.EnvVarKey, config, render, logger, repo)
	}),

	// Token handler is registered on the public router (no auth middleware required).
	fx.Provide(auth.NewTokenHandler),

	// Auth-protected route group — constructors are called here so each handler
	// is provided once as a Route value, not separately as its concrete type.
	fx.Provide(
		fx.Annotate(entry.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(box.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(export.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(prefix.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(tracking.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(NewStaticHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
	),
)

func NewServeMux(lc fx.Lifecycle, logger *zap.Logger) *http.ServeMux {
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
