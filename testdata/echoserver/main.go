package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type RequestInfo struct {
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading request body: %v", err), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	requestInfo := RequestInfo{
		Headers: r.Header,
		Body:    string(bodyBytes),
	}

	responseBytes, err := json.Marshal(requestInfo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error marshalling response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseBytes)
}

func main() {
	port := os.Getenv("ECHOSERVER_PORT")
	if port == "" {
		port = "8080" // Default port
	}

	http.HandleFunc("/", echoHandler)
	log.Printf("Echo server listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
