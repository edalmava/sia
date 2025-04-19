package handlers

import (
	"fmt"
	"net/http"
	"os"

	"github.com/edalmava/sia/api-server/internal/auth"
	"github.com/edalmava/sia/api-server/internal/middleware"
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
	//Obtener el JSON de la request
	body := middleware.GetJsonBody(r)
	if body == nil {
		http.Error(w, "Error, incorrect body", http.StatusBadRequest)
		return
	}
	//Obtener el username del JSON
	username, ok := body["username"].(string)
	if !ok {
		http.Error(w, "Error, incorrect body", http.StatusBadRequest)
		return
	}
	//Generar el token con el username
	token, err := auth.GenerateToken(username)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Enviar el token como respuesta
	fmt.Fprintf(w, "Token: %s\n", token)
}
