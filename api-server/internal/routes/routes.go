package routes

import (
	"net/http"

	"github.com/edalmava/sia/api-server/internal/auth"
	"github.com/edalmava/sia/api-server/internal/handlers"
	"github.com/edalmava/sia/api-server/internal/middleware"
	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	r := mux.NewRouter()
	r.Use(middleware.RecoverMiddleware)
	r.Use(middleware.CorsMiddleware)
	r.Use(middleware.LoggingMiddleware)

	r.Handle("/health", http.HandlerFunc(handlers.HealthHandler)).Methods(http.MethodGet)

	// API versioning
	api := r.PathPrefix("/api/v1").Subrouter()

	// Rutas públicas
	public := api.PathPrefix("").Subrouter()
	public.HandleFunc("/", handlers.HomeHandler).Methods(http.MethodGet)
	public.Use(middleware.JsonBodyMiddleware)
	public.HandleFunc("/login", handlers.LoginHandler).Methods(http.MethodPost)

	// Rutas protegidas
	protected := api.PathPrefix("").Subrouter()
	protected.Use(auth.JwtMiddleware)

	protected.HandleFunc("/about", handlers.AboutHandler).Methods(http.MethodGet)

	//r.Handle("/about", auth.JwtMiddleware(http.HandlerFunc(handlers.AboutHandler))).Methods(http.MethodGet)
	//r.Handle("/login", middleware.JsonBodyMiddleware(http.HandlerFunc(handlers.LoginHandler))).Methods(http.MethodPost)
	return r
}
