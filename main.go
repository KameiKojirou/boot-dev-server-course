package main

import (
	"log"
	"net/http"
)

func main() {
	// Create a new ServeMux instance.
	mux := http.NewServeMux()

	// Create a new server with the ServeMux as its handler.
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Server is running on http://localhost:8080")
	// Start the server. This will block until the server exits.
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
