package middleware

import (
	"net/http"
)

func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Permitir peticiones desde cualquier origen.
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Permitir los métodos que se van a utilizar.
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		// Permitir los encabezados que se van a utilizar.
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Permitir credenciales (cookies, Authorization header)
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Si la petición es OPTIONS, devolver una respuesta vacía
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Continuar con el siguiente handler.
		next.ServeHTTP(w, r)
	})
}
