// Package entrypushd contains the entrypushd consumer wiring.
package entrypushd

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"
	googlegrpc "google.golang.org/grpc"
	"nbox/internal/auth"
	"nbox/internal/auth/approle"
	"nbox/internal/auth/awssts"
	"nbox/internal/config"
	grpcint "nbox/internal/entrypushd/grpc"
	"nbox/internal/entrypushd/handler"
	"nbox/internal/entrypushd/nboxclient"
	"nbox/internal/platform/natsbus"
)

// Module wires the entrypushd binary: NATS consumer + gRPC server with
// AppRole + AWS STS M2M authentication.
var Module = fx.Module("entrypushd",
	fx.Provide(
		NewSlog,
		// NATS connection (fan-out bus).
		func(cfg Config, lc fx.Lifecycle, log *slog.Logger) (*natsgo.Conn, error) {
			return natsbus.NewConn(cfg.NatsURL, "entrypushd", lc, log)
		},
		NewNATSSubscriber,
		handler.NewBroadcast,
		grpcint.NewBroker,
		// Snapshotter (optional, Fase 2.1.d): a real nbox.Client when
		// NboxURL is set, nil otherwise (Watch streams deltas only).
		// Reuses the shared 10s *http.Client.
		func(cfg Config, hc *http.Client, logger *slog.Logger) grpcint.Snapshotter {
			if cfg.NboxURL == "" {
				logger.Warn("snapshot disabled: ENTRYPUSHD_NBOX_URL is empty; Watch will stream deltas only")
				return nil
			}
			logger.Info("snapshot enabled", slog.String("nbox_url", cfg.NboxURL))
			return nboxclient.NewClient(cfg.NboxURL, hc)
		},
		grpcint.NewStreamServer,
		grpcint.NewServer,

		// Config source chain: env first, then DynamoDB if a table is set.
		func(cfg Config, ddb *dynamodb.Client) config.Source {
			return config.NewSourceChain(ddb, cfg.ConfigTableName)
		},

		// AppRole store (refreshing) + authenticator.
		func(src config.Source, cfg Config, lc fx.Lifecycle, log *slog.Logger) (approle.Store, error) {
			snap := config.NewSnapshot(config.KeyAppRole, src, approle.NewInMemory, cfg.ConfigTTL, log)
			if err := config.Activate(lc, snap); err != nil {
				return nil, err
			}
			return approle.NewRefreshingStore(snap), nil
		},
		fx.Annotate(
			func(s approle.Store) auth.Authenticator { return approle.New(s) },
			fx.ResultTags(`group:"authenticators"`),
		),

		// AWS STS store (refreshing) + authenticator.
		func(src config.Source, cfg Config, lc fx.Lifecycle, log *slog.Logger) (awssts.Store, error) {
			snap := config.NewSnapshot(config.KeyARNMap, src, awssts.NewInMemory, cfg.ConfigTTL, log)
			if err := config.Activate(lc, snap); err != nil {
				return nil, err
			}
			return awssts.NewRefreshingStore(snap), nil
		},
		fx.Annotate(
			func(s awssts.Store, hc *http.Client) auth.Authenticator { return awssts.New(s, hc) },
			fx.ResultTags(`group:"authenticators"`),
		),

		// HTTP client for STS forwarding. 10s timeout — STS roundtrip is
		// in the critical path of every agent connect.
		func() *http.Client {
			return &http.Client{Timeout: 10 * time.Second}
		},

		// Registry assembled from all authenticators registered in the group.
		fx.Annotate(
			func(auths []auth.Authenticator) auth.Registry {
				return auth.NewRegistry(auths...)
			},
			fx.ParamTags(`group:"authenticators"`),
		),

		// HMACKey for the M2M JWT signer (named type to avoid []byte
		// collision with other providers).
		func(cfg Config) grpcint.HMACKey { return grpcint.HMACKey(cfg.HmacSecretKey) },

		// AppRoleDisabled kill-switch (named type for unambiguous fx wiring).
		func(cfg Config) grpcint.AppRoleDisabled { return grpcint.AppRoleDisabled(cfg.ApproleDisabled) },

		// Auth interceptor.
		grpcint.NewAuthInterceptor,

		// Existing adapters.
		func(b *grpcint.Broker) handler.Publisher { return b },
		func(cfg Config) grpcint.ListenAddr { return grpcint.ListenAddr(cfg.GRPCListen) },
	),
	fx.Invoke(func(*googlegrpc.Server) {}),
	fx.Invoke(StartConsumer),
)
