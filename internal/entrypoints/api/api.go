package api

import (
	"nbox/internal/entrypoints/api/auth"
	"nbox/internal/entrypoints/api/handlers"
	"nbox/internal/entrypoints/api/health"
	"nbox/internal/entrypoints/api/response"
	"net/http"
	"time"

	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	_ "nbox/docs" // Importa los documentos generados por swag
)

type Api struct {
	Engine http.Handler
}

// NewApi
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
func NewApi(
	box *handlers.BoxHandler,
	entry *handlers.EntryHandler,
	healthCheck *health.Health,
	static *handlers.StaticHandler,
	authn *auth.Authn,
) *Api {

	corsConfig := cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	})

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	//r.Use(authn.Handler())
	r.Use(corsConfig)

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"), //The url pointing to API definition
	))

	r.NotFound(response.NotFound)
	r.MethodNotAllowed(response.MethodNotAllowed)

	r.Get("/health", healthCheck.Healthy)

	r.Post("/api/auth/token", authn.TokenHandler)

	r.Group(func(r chi.Router) {
		r.Use(authn.Handler())
		//r.Use(auth.NewBasicAuthFromEnv("api", application.PrefixBasicAuthCredentials))
		r.Post("/api/box", box.UpsertBox)
		r.Get("/api/box", box.List)
		r.Head("/api/box/{service}/{stage}/{template}", box.Exist)
		r.Get("/api/box/{service}/{stage}/{template}", box.Retrieve)
		r.Get("/api/box/{service}/{stage}/{template}/build", box.Build)
		r.Get("/api/box/{service}/{stage}/{template}/vars", box.ListVars)

		r.Post("/api/entry", entry.Upsert)
		r.Get("/api/entry/key", entry.GetByKey)
		r.Get("/api/entry/prefix", entry.ListByPrefix)
		r.Delete("/api/entry/key", entry.DeleteKey)
		r.Get("/api/track/key", entry.Tracking)

		r.Get("/api/static/environments", static.Environments)

	})

	return &Api{
		Engine: r,
	}
}
