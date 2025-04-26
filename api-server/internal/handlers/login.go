package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/edalmava/sia/api-server/internal/auth"
	"github.com/edalmava/sia/api-server/internal/db/models"
	"github.com/edalmava/sia/api-server/internal/middleware"
)

type LoginResponse struct {
	Token string      `json:"token"`
	User  models.User `json:"user"`
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	//Obtener el JSON de la request
	body := middleware.GetJsonBody(r)
	if body == nil {
		http.Error(w, "Error, incorrect body", http.StatusBadRequest)
		return
	}

	var user models.User

	//Obtener el username del JSON
	username, ok := body["username"].(string)
	if !ok {
		http.Error(w, "Error, incorrect body", http.StatusBadRequest)
		return
	}

	user.Username = username

	//Generar el token con el username
	token, err := auth.GenerateToken(user)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Enviar el token como respuesta
	//fmt.Fprintf(w, "Token: %s\n", token)
	// Devolver respuesta con token y datos del usuario
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Token: token,
		User:  user,
	})
}
