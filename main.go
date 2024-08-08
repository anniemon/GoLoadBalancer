package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	nodes := []NodeParams{
		{ID: 1, URL: "http://localhost:8081", ReqLimit: 2, BodyLimit: 123},
		{ID: 2, URL: "http://localhost:8082", ReqLimit: 5, BodyLimit: 2 * 1024 * 1024},
		{ID: 3, URL: "http://localhost:8083", ReqLimit: 7, BodyLimit: 1 * 1024 * 1024},
	}

	lb := &LoadBalancer{}

	for _, nodeParams := range nodes {
		go startBackendServer(nodeParams.ID, nodeParams.URL)
		lb.AddNode(NewNode(nodeParams))
	}

	time.Sleep(1 * time.Second)

	http.Handle("/", lb)
	fmt.Println("Load Balancer running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
