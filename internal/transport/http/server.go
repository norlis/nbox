package transporthttp

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/norlis/httpgate/authz/opa"
	"github.com/norlis/httpgate/health"
	"github.com/norlis/httpgate/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/fx"
	_ "nbox/docs"
	"nbox/internal/application"
	auth "nbox/internal/auth"
	"nbox/internal/nbox"
	authmw "nbox/internal/transport/http/middleware"
)

// Route is the interface implemented by all domain handlers.
type Route interface {
	Register(mux *http.ServeMux)
}

type Params struct {
	fx.In
	Router       *http.ServeMux
	Authn        *authmw.Authn
	TokenHandler *auth.TokenHandler
	Status       *health.Status
	Readiness    *health.Readiness
	OPA          *opa.Client
	Config       *nbox.Config
	Slog         *slog.Logger
	Routes       []Route `group:"routes"`
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
// maxRequestBodyBytes caps incoming request bodies to 1 MiB.
const maxRequestBodyBytes = 1 << 20

func NewServer(params Params) {
	base := []middleware.Middleware{
		middleware.TraceID(
			middleware.WithHeaderName("x-transaction-id"),
			middleware.WithLogger(params.Slog),
		),
		middleware.InterceptStatus(
			middleware.WithIntercept(http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusInternalServerError),
			middleware.WithMessage(http.StatusNotFound, "resource not found"),
			middleware.WithMessage(http.StatusMethodNotAllowed, "method is not allowed for this resource."),
		),
		middleware.Recover(params.Slog),
		authmw.MaxBodyBytes(maxRequestBodyBytes),
		middleware.RequestLogger(params.Slog, middleware.WithSkipPaths("/status", "/live", "/ready")),
		middleware.AllowAll(),
	}

	csrfOpts := make([]middleware.CSRFOption, 0, len(params.Config.CSRFTrustedOrigins))
	for _, o := range params.Config.CSRFTrustedOrigins {
		csrfOpts = append(csrfOpts, middleware.WithTrustedOrigin(o))
	}
	base = append(base, middleware.CSRFProtect(csrfOpts...))

	use := middleware.New(base...)

	params.Router.Handle("GET /status", use.Then(params.Status))
	params.Router.Handle("GET /health", use.Then(health.NewProbe(nil)))
	params.Router.Handle("GET /ready", use.Then(params.Readiness))
	params.Router.Handle("GET /swagger/", use.Then(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)))
	params.Router.Handle("POST /api/auth/token", use.Then(http.HandlerFunc(params.TokenHandler.Token)))

	api := http.NewServeMux()
	for _, route := range params.Routes {
		route.Register(api)
	}

	useAuth := middleware.New(base...).Append(
		authmw.CanonicalizeKey(),
		params.Authn.Handler(),
		middleware.Authorize(params.OPA, fromContextExtractor),
	)
	params.Router.Handle("/api/", useAuth.Then(api))
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
