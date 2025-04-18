package main

import (
	"log"
	"net/http"
	"os"

	"github.com/edalmava/sia/api-server/internal/routes"
)

func main() {
	log.Print("starting server...")
	r := routes.NewRouter()

	// Determine port for HTTP service.
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
		log.Printf("defaulting to port %s", port)
	}

	// Start HTTP server.
	log.Printf("listening on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
