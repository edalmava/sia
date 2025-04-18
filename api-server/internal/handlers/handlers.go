package handlers

import (
	"fmt"
	"net/http"
	"os"

	"github.com/edalmava/sia/api-server/internal/auth"
)

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	name := os.Getenv("NAME")
	if name == "" {
		name = "World"
	}
	fmt.Fprintf(w, "Hello %s!\n", name)
}

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "About us\n")
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Enviar el token como respuesta
	fmt.Fprintf(w, "Token: %s\n", token)
}
