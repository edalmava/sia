package middleware

import (
	"log"
	"net/http"
	"os"
	"time"
)

func LoggingMiddleware(next http.Handler) http.Handler {
	// Ensure the logs directory exists
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		os.Mkdir("logs", 0755)
	}

	// Open the log file
	logFile, err := os.OpenFile("logs/api.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	// Create a new logger that writes to the file
	logger := log.New(logFile, "", log.LstdFlags)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r) // Continue with the next handler
		duration := time.Since(start)
		logger.Printf("%s %s %s %s", r.Method, r.RequestURI, r.RemoteAddr, duration)
	})
}
