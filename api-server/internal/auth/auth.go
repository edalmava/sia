package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/edalmava/sia/api-server/internal/db/models"
	"github.com/golang-jwt/jwt/v5"
)

// jwtKey es la clave secreta que se usa para firmar los tokens.
// En un entorno real, esta clave NO debería estar hardcodeada en el código.
// Debería obtenerse de una variable de entorno o de un archivo de configuración.
var jwtKey = []byte("Edalmava-2025-Evaluacion") // Recuerda cambiar esto en producción

// Claims es una estructura que representa las "claims" que irán dentro del token.
// Se define el username y las claims de jwt.
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type contextKey string

const (
	usernameContextKey contextKey = "username"
	roleContextKey     contextKey = "role"
)

// JwtMiddleware es el middleware que se encarga de verificar la validez del token JWT.
func JwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// El formato del header debe ser "Bearer {token}"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Authorization header format must be Bearer {token}", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims := &Claims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Verificar que el método de firma sea el esperado
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("método de firma inesperado: %v", token.Header["alg"])
			}
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

		// Guardar información del usuario en el contexto
		ctx := context.WithValue(r.Context(), usernameContextKey, claims.Username)
		ctx = context.WithValue(ctx, roleContextKey, claims.Role)

		// Llamar al siguiente handler con el nuevo contexto
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GenerateToken genera un nuevo token JWT.
func GenerateToken(user models.User) (string, error) {
	expirationTime := time.Now().Add(1 * time.Hour)
	claims := &Claims{
		Username: user.Username,
		Role:     user.Role,
		// Aquí puedes agregar más claims si lo deseas
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// GetUsernameFromContext extrae el nombre de usuario del contexto
func GetUsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(usernameContextKey).(string)
	return username, ok
}

// GetRoleFromContext extrae el rol del contexto
func GetRoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(roleContextKey).(string)
	return role, ok
}
