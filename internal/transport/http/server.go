package transporthttp

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/middleware"
	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"github.com/norlis/httpgate/pkg/adapter/opa"
	"github.com/norlis/httpgate/pkg/application/health"
	"github.com/norlis/httpgate/pkg/port"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
	_ "nbox/docs"
	"nbox/internal/application"
	auth "nbox/internal/auth"
	"nbox/internal/event/bus"
	platformaws "nbox/internal/platform/aws"
	authmw "nbox/internal/transport/http/middleware"
)

// Route is the interface implemented by all domain handlers.
type Route interface {
	Register(mux *http.ServeMux)
}

type Params struct {
	fx.In
	Router          *http.ServeMux
	Authn           *authmw.Authn
	TokenHandler    *auth.TokenHandler
	Status          *health.Status
	Render          presenters.Presenters
	Logger          *zap.Logger
	S3Checker       *platformaws.S3Checker
	DynamoDBChecker *platformaws.DynamoDBChecker
	SSMChecker      *platformaws.SSMChecker
	EventBroker     *bus.Memory
	UI              *UIHandler
	Routes          []Route `group:"routes"`
}

// NewServer
// @title           nbox API
// @version         2.0
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
func NewServer(params Params) {
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
		PoliciesPath: "policies/authz",
		DataFiles:    []string{},
	}

	authz, err := opa.NewOpaSdkClientFromConfig(context.Background(), opaConfig, params.Logger)
	if err != nil {
		log.Fatalf("The OPA client could not be initialized: %v", err)
	}

	use := middleware.Chain(base...)

	params.Router.Handle("GET /status", use(params.Status))
	params.Router.Handle("GET /health", use(health.NewProbe(nil)))
	params.Router.Handle("GET /ready", use(health.NewProbe(map[string]port.Checker{
		"s3":       params.S3Checker,
		"ssm":      params.SSMChecker,
		"dynamodb": params.DynamoDBChecker,
	})))

	params.Router.Handle("GET /swagger/", use(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)))

	params.Router.Handle("POST /api/auth/token", use(http.HandlerFunc(params.TokenHandler.Token)))
	params.Router.Handle("GET /api/events", params.EventBroker)
	params.Router.HandleFunc("GET /events", params.UI.EventsPage)
	params.Router.Handle("GET /assets/", params.UI.ServeAssets())

	api := http.NewServeMux()

	for _, route := range params.Routes {
		route.Register(api)
	}

	useAuth := middleware.Chain(
		append(
			base,
			[]middleware.Middleware{
				params.Authn.Handler(),
				middleware.AuthorizationMiddleware(authz, fromContextExtractor),
			}...,
		)...,
	)
	params.Router.Handle("/api/", useAuth(api))
}

func fromContextExtractor(r *http.Request) (map[string]any, error) {
	user, ok := application.UserFromContext(r.Context())
	if !ok {
		return nil, errors.New("no user found in context")
	}

	return map[string]any{
		"roles": user.Roles,
	}, nil
}
