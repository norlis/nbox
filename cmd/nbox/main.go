package main

import (
	"flag"
	"fmt"
	"log"
	"nbox/internal/adapters/storage"
	"os"

	"nbox/internal/adapters/amazonaws"
	"nbox/internal/adapters/persistence"
	"nbox/internal/adapters/sse"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/entrypoints/api/auth"
	"nbox/internal/entrypoints/api/handlers"
	"nbox/internal/entrypoints/httpapi"
	"nbox/internal/usecases"
	"nbox/pkg/logger"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	status "github.com/norlis/httpgate/pkg/application/health"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
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
			return &fxevent.ZapLogger{Logger: log}
		}),
		fx.Provide(amazonaws.NewAwsConfig),
		fx.Provide(amazonaws.NewS3Client),
		fx.Provide(amazonaws.NewDynamodbClient),
		fx.Provide(amazonaws.NewSsmClient),

		// Checkers (health | status | live | ready)
		fx.Provide(amazonaws.NewS3Checker),
		fx.Provide(amazonaws.NewDynamoDBChecker),
		fx.Provide(amazonaws.NewSSMChecker),

		// Adapters
		fx.Provide(amazonaws.NewS3TemplateStore),

		// Adapters New v2
		fx.Provide(storage.NewDynamodbKit),
		fx.Provide(storage.NewDynamoPrefixConfigRepository),
		fx.Provide(storage.NewDynamoDBTracking),

		fx.Provide(storage.NewDynamoDBBackend),             // Backend DynamoDB (e Indice Global)
		fx.Provide(storage.NewParameterStoreBackend),       // Backend SSM Standard
		fx.Provide(storage.NewParameterStoreSecureBackend), // Backend SSM Secure

		// gateway manager storages
		fx.Provide(func(
			l *zap.Logger,
			pr domain.PrefixConfigRepository,
			ddb *storage.DynamoDBBackend,
			ssm *storage.ParameterStoreBackend,
			ssmSec *storage.ParameterStoreSecureBackend,
		) domain.EntryManager {
			gw := storage.NewStorageGateway(ddb, pr, l)
			gw.RegisterBackend(ddb)
			gw.RegisterBackend(ssm)
			gw.RegisterBackend(ssmSec)
			return gw
		}),
		fx.Decorate(usecases.NewEntryManagerWithTracking),


		// Handlers
		fx.Provide(handlers.NewEntryHandler),
		fx.Provide(handlers.NewBoxHandler),
		fx.Provide(handlers.NewStaticHandler),
		fx.Provide(handlers.NewUIHandler),
		fx.Provide(handlers.NewExportHandler),
		fx.Provide(handlers.NewPrefixConfigHandler),
		fx.Provide(handlers.NewTrackHandler),


		// Use case
		fx.Provide(usecases.NewPathUseCase),
		//fx.Provide(usecases.NewEntryUseCase),
		fx.Provide(usecases.NewBox),
		fx.Provide(usecases.NewExportUseCase),

		//fx.Decorate(usecases.NewEntryUseCaseWithEvents),
		fx.Provide(usecases.NewEventUseCase),

		// sse
		fx.Provide(sse.NewEventBroker),
		fx.Provide(sse.NewInMemoryEventPublisher),

		// config
		fx.Provide(application.NewConfigFromEnv),
		fx.Provide(func() *status.Status {
			version := application.GitHash
			if version == "" {
				version = "unknown.dev"
			}
			return status.NewStatus(version)
		}),
		fx.Provide(presenters.NewPresenters),
		fx.Provide(httpapi.NewHttpServerMux),
		fx.Provide(func() domain.UserRepository {
			credentials := os.Getenv(application.EnvCredentials)
			repo, err := persistence.NewInMemoryUserRepository([]byte(credentials))
			if err != nil {
				log.Fatal(err)
			}
			return repo
		}),
		fx.Provide(func(config *application.Config, render presenters.Presenters, logger *zap.Logger, repo domain.UserRepository) *auth.Authn {
			return auth.NewAuthn(application.EnvCredentials, config, render, logger, repo)
		}),
		fx.Invoke(httpapi.NewHttpApi),
	)

	if err := app.Err(); err != nil {
		log.Panicf("Error en la inicialización de la aplicación FX: %v\n", err)
	}

	app.Run()
}
