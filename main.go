package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jonboulle/clockwork"
)

func main() {
	nodeParams := []NodeParams{
			{ID: 1, URL: "http://localhost:8081", ReqLimit: 2, BodyLimit: 123},
			{ID: 2, URL: "http://localhost:8082", ReqLimit: 5, BodyLimit: 2 * 1024 * 1024},
			{ID: 3, URL: "http://localhost:8083", ReqLimit: 7, BodyLimit: 1 * 1024 * 1024},
	}

	nodes := make([]*Node, len(nodeParams))
	for i, params := range nodeParams {
		go startBackendServer(params.ID, params.URL)
		nodes[i] = NewNode(params)
	}

	nodeManager := NewSafeNodeManager(nodes, clockwork.NewRealClock())
	nodeManager.StartPeriodicTasks()
	lb := NewLoadBalancer(nodeManager)

	time.Sleep(1 * time.Second)

	http.Handle("/", lb)
	fmt.Println("Load Balancer running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
