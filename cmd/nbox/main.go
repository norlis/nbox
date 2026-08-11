package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	natsgo "github.com/nats-io/nats.go"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"nbox/internal/application"
	auth "nbox/internal/auth"
	authstore "nbox/internal/auth/store"
	"nbox/internal/box"
	boxstore "nbox/internal/box/store"
	"nbox/internal/config"
	"nbox/internal/entry"
	entrystore "nbox/internal/entry/store"
	event "nbox/internal/event"
	"nbox/internal/event/publisher"
	"nbox/internal/export"
	"nbox/internal/nbox"
	platformaws "nbox/internal/platform/aws"
	"nbox/internal/platform/natsbus"
	"nbox/internal/prefix"
	prefixstore "nbox/internal/prefix/store"
	"nbox/internal/tracking"
	trackingstore "nbox/internal/tracking/store"
	transporthttp "nbox/internal/transport/http"
	"nbox/pkg/logger"
)

// banner
// https://patorjk.com/software/taag/#p=display&f=Doh&t=NBOX
const banner = `

NNNNNNNN        NNNNNNNNBBBBBBBBBBBBBBBBB        OOOOOOOOO     XXXXXXX       XXXXXXX
N:::::::N       N::::::NB::::::::::::::::B     OO:::::::::OO   X:::::X       X:::::X
N::::::::N      N::::::NB::::::BBBBBB:::::B  OO:::::::::::::OO X:::::X       X:::::X
N:::::::::N     N::::::NBB:::::B     B:::::BO:::::::OOO:::::::OX::::::X     X::::::X
N::::::::::N    N::::::N  B::::B     B:::::BO::::::O   O::::::OXXX:::::X   X:::::XXX
N:::::::::::N   N::::::N  B::::B     B:::::BO:::::O     O:::::O   X:::::X X:::::X
N:::::::N::::N  N::::::N  B::::BBBBBB:::::B O:::::O     O:::::O    X:::::X:::::X
N::::::N N::::N N::::::N  B:::::::::::::BB  O:::::O     O:::::O     X:::::::::X
N::::::N  N::::N:::::::N  B::::BBBBBB:::::B O:::::O     O:::::O     X:::::::::X
N::::::N   N:::::::::::N  B::::B     B:::::BO:::::O     O:::::O    X:::::X:::::X
N::::::N    N::::::::::N  B::::B     B:::::BO:::::O     O:::::O   X:::::X X:::::X
N::::::N     N:::::::::N  B::::B     B:::::BO::::::O   O::::::OXXX:::::X   X:::::XXX
N::::::N      N::::::::NBB:::::BBBBBB::::::BO:::::::OOO:::::::OX::::::X     X::::::X
N::::::N       N:::::::NB:::::::::::::::::B  OO:::::::::::::OO X:::::X       X:::::X
N::::::N        N::::::NB::::::::::::::::B     OO:::::::::OO   X:::::X       X:::::X
NNNNNNNN         NNNNNNNBBBBBBBBBBBBBBBBB        OOOOOOOOO     XXXXXXX       XXXXXXX

`

// eventModule wires the event publisher. The flag is read eagerly (outside fx)
// because it decides which providers exist: when disabled the *natsgo.Conn
// provider is omitted, so fx never dials NATS. fx.Private keeps the conn scoped
// to this module.
func eventModule(pubCfg publisher.Config) fx.Option {
	if !pubCfg.Enabled {
		return fx.Module("events",
			fx.Provide(func(log *zap.Logger) event.Publisher {
				return publisher.NewNoop(log)
			}),
		)
	}
	return fx.Module("events",
		fx.Supply(pubCfg),
		fx.Provide(
			func(cfg *nbox.Config, lc fx.Lifecycle, log *slog.Logger) (*natsgo.Conn, error) {
				return natsbus.NewConn(cfg.NatsURL, "nbox", lc, log)
			},
			fx.Private,
		),
		fx.Provide(func(pubCfg publisher.Config, cfg *nbox.Config, nc *natsgo.Conn, log *zap.Logger) (event.Publisher, error) {
			return publisher.New(pubCfg, nc, cfg.EventSubject(), log)
		}),
	)
}

func main() {
	fmt.Print(banner)

	var listenAddress, listenPort string
	flag.StringVar(&listenPort, "port", "7337", "--port=7337")
	flag.StringVar(&listenAddress, "address", "", "--address=0.0.0.0")
	flag.Parse()

	pubCfg, err := publisher.LoadConfig()
	if err != nil {
		log.Fatalf("nbox: load publisher config: %v", err)
	}

	app := fx.New(
		fx.Provide(logger.LoadConfig),
		fx.Provide(logger.NewLogger),
		fx.Provide(logger.NewSlog),
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log.WithOptions(zap.IncreaseLevel(zapcore.WarnLevel))}
		}),

		// Core infrastructure
		application.Module,
		nbox.Module,
		platformaws.Module,

		// Domain modules (business logic)
		auth.Module,
		entry.Module,
		box.Module,
		export.Module,
		tracking.Module,
		prefix.Module,

		// Store wiring — kept here because store sub-packages import their domain
		// packages, making it impossible to wire them inside the domain module.go files.
		fx.Provide(func(cfg *nbox.Config, ddb *dynamodb.Client) config.Source {
			return config.NewSourceChain(ddb, cfg.ConfigTableName)
		}),
		fx.Provide(func(src config.Source, cfg *nbox.Config, lc fx.Lifecycle, log *slog.Logger) (auth.Store, error) {
			snap := config.NewSnapshot(config.KeyBasicAuth, src, authstore.NewInMemory, cfg.ConfigTTL, log)
			if err := config.Activate(lc, snap); err != nil {
				return nil, err
			}
			return authstore.NewRefreshingStore(snap), nil
		}),
		fx.Provide(boxstore.NewS3),
		fx.Provide(func(
			src config.Source,
			cfg *nbox.Config,
			proc *entry.Processor,
			ddb *dynamodb.Client,
			lc fx.Lifecycle,
			log *slog.Logger,
		) (prefix.Store, error) {
			parse := func(raw []byte) (*prefixstore.PrefixIndex, error) {
				return prefixstore.ParseIndex(raw, proc)
			}
			snap := config.NewSnapshot(config.KeyPrefixConfig, src, parse, cfg.ConfigTTL, log)
			if err := config.Activate(lc, snap); err != nil {
				return nil, err
			}
			return prefixstore.NewConfigBacked(snap, config.NewAdminStore(ddb, cfg.ConfigTableName)), nil
		}),
		fx.Provide(trackingstore.NewDynamoDB),
		fx.Provide(entrystore.NewDynamoDB),
		fx.Provide(entrystore.NewSSM),
		fx.Provide(func(base *entrystore.SSM, logger *zap.Logger) *entrystore.SSMSecure {
			return entrystore.NewSSMSecure(base, logger)
		}),
		fx.Provide(func(
			l *zap.Logger,
			pr prefix.Store,
			ddb *entrystore.DynamoDB,
			ssm *entrystore.SSM,
			ssmSec *entrystore.SSMSecure,
		) entry.Manager {
			gw := entrystore.NewGateway(ddb, pr, l)
			gw.RegisterBackend(ddb)
			gw.RegisterBackend(ssm)
			gw.RegisterBackend(ssmSec)
			return gw
		}),
		eventModule(pubCfg),

		// Tracking recorder wraps entry.Manager with audit trail + event dispatch.
		// Must be at top-level scope so it applies to all consumers (box, export, handlers).
		fx.Decorate(tracking.NewRecorder),

		// HTTP transport
		fx.Supply(transporthttp.ServerConfig{Address: listenAddress, Port: listenPort}),
		transporthttp.Module,
	)

	if err := app.Err(); err != nil {
		log.Panicf("Error en la inicialización de la aplicación FX: %v\n", err)
	}

	app.Run()
}
