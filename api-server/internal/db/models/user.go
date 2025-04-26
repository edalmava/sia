package models

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // No mostrar en respuestas JSON
	Role     string `json:"role"`               // "admin", "teacher" o "student"
	Active   bool   `json:"active"`             // Estado del usuario (activo/inactivo)
}
