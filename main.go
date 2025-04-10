package main

import (
  "log"
  "net/http"
)

func main() {
  // Create a new ServeMux instance.
  mux := http.NewServeMux()

  // Create a FileServer handler to serve files from the current directory.
  fs := http.FileServer(http.Dir("."))

  // Register the FileServer handler for the root path.
  // The FileServer will automatically serve index.html for GET "/"
  mux.Handle("/", fs)

  // Create a new server using the ServeMux as its handler.
  server := &http.Server{
    Addr:    ":8080",
    Handler: mux,
  }

  log.Println("Server is running on http://localhost:8080")
  // Start the server. This call blocks until the server exits.
  if err := server.ListenAndServe(); err != nil {
    log.Fatal(err)
  }
}
