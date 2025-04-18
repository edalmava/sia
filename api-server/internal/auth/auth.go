package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtKey es la clave secreta que se usa para firmar los tokens.
// En un entorno real, esta clave NO debería estar hardcodeada en el código.
// Debería obtenerse de una variable de entorno o de un archivo de configuración.
var jwtKey = []byte("mysecretkey") // Recuerda cambiar esto en producción

// Claims es una estructura que representa las "claims" que irán dentro del token.
// Se define el username y las claims de jwt.
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JwtMiddleware es el middleware que se encarga de verificar la validez del token JWT.
func JwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tokenString := strings.Split(authHeader, " ")[1]
		claims := &Claims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !token.Valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateToken genera un nuevo token JWT.
func GenerateToken() (string, error) {
	// Definir el tiempo de expiración del token (ejemplo: 1 hora)
	expirationTime := time.Now().Add(1 * time.Hour)

	// Crear las claims (datos que irán dentro del token)
	claims := &Claims{
		Username: "testuser", // En un escenario real, esto se obtendría del usuario
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	// Crear el token con las claims y el algoritmo de firma
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Firmar el token con la clave secreta
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}
