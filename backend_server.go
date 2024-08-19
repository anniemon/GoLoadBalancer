package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
)

func startBackendServer(id int, targetURL string) {
	url, err := url.Parse(targetURL)
	if err != nil {
		log.Fatalf("Failed to parse target URL %s: %v", targetURL, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "node%d on port %s\n", id, url)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter,r *http.Request) {
		// fail by 30% chance for simulation
		if rand.Float32() < 0.3 {
			http.Error(w, "Simulated failure", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", strings.Split(targetURL, ":")[2]),
		Handler: mux,
	}
	log.Printf("Starting node %d on port %d", id, 8080+id)
	log.Fatal(server.ListenAndServe())
}
