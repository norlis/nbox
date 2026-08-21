package transporthttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	logfields2 "github.com/norlis/event-driven/pkg/kit/logfields"
	"github.com/norlis/httpgate/authz/opa"
	"github.com/norlis/httpgate/health"
	"github.com/norlis/httpgate/logging"
	"github.com/norlis/httpgate/server"
	"go.uber.org/fx"
	auth "nbox/internal/auth"
	"nbox/internal/box"
	"nbox/internal/entry"
	"nbox/internal/export"
	"nbox/internal/me"
	"nbox/internal/nbox"
	platformaws "nbox/internal/platform/aws"
	"nbox/internal/prefix"
	"nbox/internal/tracking"
	authmw "nbox/internal/transport/http/middleware"
	"nbox/internal/transport/httpx"
)

var Module = fx.Module("transport.http",
	fx.Provide(httpx.New),
	fx.Provide(NewReadiness),
	fx.Provide(NewOPAClient),
	fx.Provide(NewServeMux),
	fx.Invoke(NewServer),

	fx.Provide(func(config *nbox.Config, render *httpx.Render, logger *slog.Logger, repo auth.Store) *authmw.Authn {
		return authmw.NewAuthn(config, render, logger, repo)
	}),

	fx.Provide(auth.NewTokenHandler),

	fx.Provide(
		fx.Annotate(entry.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(box.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(export.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(prefix.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(tracking.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(me.NewHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
		fx.Annotate(NewStaticHandler, fx.As(new(Route)), fx.ResultTags(`group:"routes"`)),
	),
)

// NewReadiness builds the readiness probe (with backend checkers) wrapped in a
// drain gate. The same *health.Readiness is served at /ready and flipped to
// draining during graceful shutdown (see NewServeMux).
func NewReadiness(
	s3 *platformaws.S3Checker,
	ssm *platformaws.SSMChecker,
	ddb *platformaws.DynamoDBChecker,
) *health.Readiness {
	probe := health.NewProbe(map[string]health.Checker{
		"s3":       s3,
		"ssm":      ssm,
		"dynamodb": ddb,
	})
	return health.NewReadiness(probe)
}

// NewOPAClient initializes the OPA enforcer used by both the Authorize
// middleware and the /api/me/permissions endpoint.
func NewOPAClient(logger *slog.Logger) (*opa.Client, error) {
	return opa.New(context.Background(), opa.Config{
		Query:        "data.authz.allow",
		PoliciesPath: "policies/authz",
		DataFiles:    []string{},
	}, opa.WithLogger(logger))
}

// ServerConfig holds the HTTP listen address. It is supplied from CLI flags in
// cmd/nbox/main.go via fx.Supply, replacing the former package-level globals.
type ServerConfig struct {
	Address string
	Port    string
}

func NewServeMux(lc fx.Lifecycle, logger *slog.Logger, ready *health.Readiness, scfg ServerConfig) *http.ServeMux {
	mux := http.NewServeMux()
	addr := net.JoinHostPort(scfg.Address, scfg.Port)

	srv := server.New(mux, server.WithAddr(addr))

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// httpgate's own "server listening" log only fires via server.Run(); this
			// hook calls ListenAndServe directly, so we log the bound address ourselves.
			logger.Info("server listening", slog.String(logfields2.KeyServerAddress, addr))
			go func() {
				if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.ErrorContext(ctx, "http serve error", logging.Err(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.InfoContext(ctx, "server draining")
			ready.MarkDraining()
			shutdownCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				logger.ErrorContext(ctx, "shutdown error", logging.Err(err))
				return fmt.Errorf("failed to shutdown HTTP server: %w", err)
			}
			logger.InfoContext(ctx, "server stopped")
			return nil
		},
	})
	return mux
}
