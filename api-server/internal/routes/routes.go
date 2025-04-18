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

	r.HandleFunc("/", handlers.HomeHandler).Methods("GET")
	r.Handle("/about", auth.JwtMiddleware(http.HandlerFunc(handlers.AboutHandler))).Methods("GET")
	r.HandleFunc("/login", handlers.LoginHandler).Methods("GET")
	return r
}
