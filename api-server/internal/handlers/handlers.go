package handlers

import (
	"fmt"
	"net/http"
	"os"
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
