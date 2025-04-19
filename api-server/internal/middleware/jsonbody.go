package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// jsonBodyContextKey es un tipo para la clave de contexto
type jsonBodyContextKey string

// JsonBodyMiddleware es el middleware que analiza el body de la petición si es JSON
func JsonBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Solo procesar peticiones con Content-Type: application/json
		if r.Header.Get("Content-Type") != "application/json" {
			next.ServeHTTP(w, r)
			return
		}

		// Leer el body de la petición
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading request body: %v", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Decodificar el JSON
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			http.Error(w, fmt.Sprintf("Error unmarshaling JSON: %v", err), http.StatusBadRequest)
			return
		}

		// Agregar el JSON decodificado al contexto de la petición
		ctx := r.Context()
		ctx = context.WithValue(ctx, jsonBodyContextKey("jsonBody"), data)
		r = r.WithContext(ctx)

		// Llamar al siguiente handler
		next.ServeHTTP(w, r)
	})
}

// GetJsonBody recupera el json del contexto
func GetJsonBody(r *http.Request) map[string]interface{} {
	if body, ok := r.Context().Value(jsonBodyContextKey("jsonBody")).(map[string]interface{}); ok {
		return body
	}
	return nil
}
