package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"

	_ "nbox/docs"
	"nbox/internal/adapters/amazonaws"
	"nbox/internal/adapters/sse"
	"nbox/internal/application"
	"nbox/internal/entrypoints/api/auth"
	"nbox/internal/entrypoints/api/handlers"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/middleware"
	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"github.com/norlis/httpgate/pkg/adapter/opa"
	"github.com/norlis/httpgate/pkg/application/health"
	"github.com/norlis/httpgate/pkg/port"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Router              *http.ServeMux
	Box                 *handlers.BoxHandler
	Entry               *handlers.EntryHandler
	Static              *handlers.StaticHandler
	Authn               *auth.Authn
	Status              *health.Status
	Render              presenters.Presenters
	Logger              *zap.Logger
	S3Checker           *amazonaws.S3Checker
	DynamoDBChecker     *amazonaws.DynamoDBChecker
	SSMChecker          *amazonaws.SSMChecker
	EventBroker         *sse.EventBroker
	UI                  *handlers.UIHandler
	Export              *handlers.ExportHandler
	PrefixConfigHandler *handlers.PrefixConfigHandler
	Tracker             *handlers.TrackHandler
}

// NewHttpApi
// @title           nbox API
// @version         1.0
// @description     Esta es una API generada automáticamente con Swaggo.
// @termsOfService  http://swagger.io/terms/
// @contact.name   Norlis Viamonte
// @contact.url    http://www.example.com/support
// @contact.email  norlis.viamonte@gmail.com
// @host
// @BasePath  /
// @securityDefinitions.basic  BasicAuth
// @securityDefinitions.apikey BearerAuth
// @tokenUrl /api/auth/token
// @in header
// @name Authorization
// @description Bearer token authentication. Enter your JWT token in the format: Bearer {token}
// @openapi 3.0.0.
func NewHttpApi(params Params) {
	base := []middleware.Middleware{
		middleware.TraceId(middleware.WithHeaderName("x-transaction-id"), middleware.WithLogger(params.Logger)),
		middleware.APIErrorMiddleware(
			middleware.WithIntercept(http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusInternalServerError),
			middleware.WithCustomMessage(http.StatusNotFound, "resource not found"),
			middleware.WithCustomMessage(http.StatusMethodNotAllowed, "method is not allowed for this resource."),
		),
		middleware.Recover(params.Logger, params.Render),
		middleware.RequestLogger(params.Logger),
		middleware.AllowAll(params.Logger).Middleware,
	}

	opaConfig := opa.Config{
		Query:        "data.authz.allow",
		PoliciesPath: "policies/authz", // Directorio con authz.rego
		DataFiles:    []string{},
	}

	authz, err := opa.NewOpaSdkClientFromConfig(context.Background(), opaConfig, params.Logger)
	if err != nil {
		log.Fatalf("No se pudo inicializar el cliente OPA: %v", err)
	}

	use := middleware.Chain(base...)

	params.Router.Handle("GET /status", use(params.Status))
	params.Router.Handle("GET /health", use(health.NewProbe(nil)))
	// router.Handle("GET /live", use(health.NewProbe(nil)))
	params.Router.Handle("GET /ready", use(health.NewProbe(map[string]port.Checker{
		"s3":       params.S3Checker,
		"ssm":      params.SSMChecker,
		"dynamodb": params.DynamoDBChecker,
	})))

	params.Router.Handle("GET /swagger/", use(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)))

	params.Router.Handle("POST /api/auth/token", use(params.Authn.TokenHandler()))
	params.Router.Handle("GET /api/events", params.EventBroker)
	params.Router.HandleFunc("GET /events", params.UI.EventsPage)
	params.Router.Handle("GET /assets/", params.UI.ServeAssets())

	api := http.NewServeMux()

	api.HandleFunc("POST /api/box", params.Box.UpsertBox)
	api.HandleFunc("GET /api/box", params.Box.List)
	api.HandleFunc("HEAD /api/box/{service}/{stage}/{template}", params.Box.Exist)
	api.HandleFunc("GET /api/box/{service}/{stage}/{template}", params.Box.Retrieve)
	api.HandleFunc("GET /api/box/{service}/{stage}/{template}/build", params.Box.Build)
	api.HandleFunc("GET /api/box/{service}/{stage}/{template}/vars", params.Box.ListVars)

	api.HandleFunc("POST /api/entry", params.Entry.Upsert)
	api.HandleFunc("GET /api/entry/key", params.Entry.GetByKey)
	api.HandleFunc("GET /api/entry/prefix", params.Entry.ListByPrefix)
	api.HandleFunc("GET /api/entry/export", params.Export.Export)
	api.HandleFunc("DELETE /api/entry/key", params.Entry.DeleteKey)

	// TODO: change to /api/entry/resolve for backends parameterstore, parameterstore_secure
	api.HandleFunc("GET /api/entry/secret-value", params.Entry.Resolve)
	api.HandleFunc("GET /api/entry/resolve", params.Entry.Resolve)

	api.HandleFunc("POST /api/entry/lookup", params.Entry.RetrieveMany)

	api.HandleFunc("GET /api/track/key", params.Tracker.Tracking)

	// Deprecated: Use endpoint /api/prefix
	api.HandleFunc("GET /api/static/environments", params.Static.Environments)

	api.HandleFunc("GET /api/prefix/resolve", params.PrefixConfigHandler.GetByPrefix)
	api.HandleFunc("GET /api/prefix", params.PrefixConfigHandler.List)
	api.HandleFunc("GET /api/prefix/backends", params.PrefixConfigHandler.ListBackends)

	useAuth := middleware.Chain(
		append(
			base,
			[]middleware.Middleware{
				params.Authn.Handler(),
				middleware.AuthorizationMiddleware(authz, FromContextExtractor),
			}...,
		)...,
	)
	params.Router.Handle("/api/", useAuth(api))
}

func FromContextExtractor(r *http.Request) (map[string]any, error) {
	user, ok := application.UserFromContext(r.Context())
	if !ok {
		return nil, errors.New("no user found in context")
	}

	return map[string]any{
		"roles": user.Roles,
	}, nil
}
