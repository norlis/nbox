package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"nbox/internal/application"
	auth "nbox/internal/auth"
	authstore "nbox/internal/auth/store"
	"nbox/internal/box"
	boxstore "nbox/internal/box/store"
	"nbox/internal/entry"
	entrystore "nbox/internal/entry/store"
	"nbox/internal/event"
	"nbox/internal/event/bus"
	"nbox/internal/export"
	platformaws "nbox/internal/platform/aws"
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

func main() {
	fmt.Print(banner)

	flag.StringVar(&application.Port, "port", "7337", "--port=7337")
	flag.StringVar(&application.Address, "address", "", "--address=0.0.0.0")
	flag.Parse()

	app := fx.New(
		fx.Provide(logger.NewLogger),
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log.WithOptions(zap.IncreaseLevel(zapcore.WarnLevel))}
		}),

		// Core infrastructure
		application.Module,
		platformaws.Module,

		// Domain modules (business logic)
		auth.Module,
		entry.Module,
		box.Module,
		export.Module,
		event.Module,
		tracking.Module,
		prefix.Module,

		// Store wiring — kept here because store sub-packages import their domain
		// packages, making it impossible to wire them inside the domain module.go files.
		fx.Provide(func(config *application.Config) auth.Store {
			credentials := os.Getenv(config.CredentialsLoader.EnvVarKey)
			repo, err := authstore.NewInMemory([]byte(credentials))
			if err != nil {
				log.Fatal(err)
			}
			return repo
		}),
		fx.Provide(boxstore.NewS3),
		fx.Provide(prefixstore.NewDynamoDB),
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
		// Event bus: wired here because event/bus imports event (cycle).
		fx.Provide(bus.NewMemory),
		fx.Provide(func(m *bus.Memory) event.Publisher { return m }),

		// Tracking recorder wraps entry.Manager with audit trail + event dispatch.
		// Must be at top-level scope so it applies to all consumers (box, export, handlers).
		fx.Decorate(tracking.NewRecorder),

		// HTTP transport
		transporthttp.Module,
	)

	if err := app.Err(); err != nil {
		log.Panicf("Error en la inicialización de la aplicación FX: %v\n", err)
	}

	app.Run()
}
