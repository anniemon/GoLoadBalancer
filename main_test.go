package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

var (
	client    = &http.Client{}
	lbServer  *httptest.Server
	lb        *LoadBalancer
	servers   []*http.Server
	nodeParams     = []NodeParams{
		{ID: 1, URL: "http://localhost:8081", ReqLimit: 2, BodyLimit: 76},
		{ID: 2, URL: "http://localhost:8082", ReqLimit: 3, BodyLimit: 2 * 1024 * 1024},
		{ID: 3, URL: "http://localhost:8083", ReqLimit: 5, BodyLimit: 1 * 1024 * 1024},
	}
)

func startTestServer(port int) *http.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	go server.ListenAndServe()
	return server
}

func setup() {
	for _, nodeParams := range nodeParams {
		server := startTestServer(8080 + nodeParams.ID)
		servers = append(servers, server)
	}

	time.Sleep(1 * time.Second)

	lb = &LoadBalancer{}

	for _, nodeParams := range nodeParams {
		lb.AddNode(NewNode(nodeParams))
	}

	lbServer = httptest.NewServer(lb)
	println(lbServer.URL)
}


func teardown() {
	lbServer.Close()
	for _, server := range servers {
		server.Close()
	}
}

func resetRateLimits(lb *LoadBalancer) {
	for _, node := range lb.Nodes {
		node.Mutex.Lock()
		node.ReqCount = 0
		node.BodyCount = 0
		node.Mutex.Unlock()
	}
}

func resetHealthChecks(lb *LoadBalancer) {
	for _, node := range lb.Nodes {
		node.Mutex.Lock()
		node.Healthy = true
		node.Mutex.Unlock()
	}
}

func TestLoadBalancer(t *testing.T) {
	setup()
	defer teardown()

	t.Run("RoundRobin", testRoundRobin)
	t.Run("RateLimitExceeded", testRateLimitExceededByRPM)
	t.Run("NodeHealthHandling", testAllNodesUnhealthy)
	t.Run("AllNodesBusy", testAllNodesBusy)
	t.Run("RateLimitReset", testRateLimitReset)
}

// Should work by round robin
func testRoundRobin(t *testing.T) {
	next := lb.Next
	for i := 0; i <= 4; i++ {
		node := lb.Nodes[next]

		if node.ID != next + 1 {
			t.Errorf("Round robin not working %d, %d", node.ID, next)
		}
		next = (next + 1) % len(lb.Nodes)
	}
}

// Should hit rate limit by requests per minute for each node
func testRateLimitExceededByRPM(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(lbServer.URL)
			if err != nil {
				t.Errorf("Failed to send request: %v", err)
				return
			}
			resp.Body.Close()
			}()
	}
	wg.Wait()

	if lb.Nodes[0].ReqCount <  lb.Nodes[0].ReqLimit {
		t.Errorf("Rate Limit should be hit; RPM %d/%d", lb.Nodes[0].ReqCount, lb.Nodes[0].ReqLimit)
	}
	t.Logf("Node2 ReqCount:%d, ReqLimit:%d", lb.Nodes[1].ReqCount, lb.Nodes[1].ReqLimit)
	t.Logf("Node3 ReqCount:%d, ReqLimit:%d", lb.Nodes[2].ReqCount, lb.Nodes[2].ReqLimit)
}

// TODO: Should hit rate limit by request body size per minute for each node
func testRateLimitExceededByBPM(t *testing.T) {
}

// Should return 503 when all nodes are unhealthy
func testAllNodesUnhealthy(t *testing.T) {
	resetRateLimits(lb)

	lb.Nodes[0].Healthy = false
	lb.Nodes[1].Healthy = false
	lb.Nodes[2].Healthy = false

	resp, err := client.Get(lbServer.URL)

	if err != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
		t.Logf("Err: %v", err)
	}
	resp.Body.Close()
}

// Should return 503 when all nodes have hit rate limits
func testAllNodesBusy(t *testing.T) {
	resetHealthChecks(lb)

	for _, node := range lb.Nodes {
		node.ReqCount = node.ReqLimit
	}

	resp, err := client.Get(lbServer.URL)
	if err != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
		t.Logf("Err: %v", err)
	}
	resp.Body.Close()
}

// Should reset RPM and BPM for each node every minute
func testRateLimitReset(t *testing.T) {
	time.Sleep(61 * time.Second) // TODO: replace with fake timer

	for i := 0; i < 3; i++ {
		if lb.Nodes[i].ReqCount > 0 {
			t.Errorf("ReqCount should reset to %d, got %d", 0, lb.Nodes[i].ReqCount)
		}
		if lb.Nodes[i].BodyCount > 0 {
			t.Errorf("ReqCount should reset to %d, got %d", 0, lb.Nodes[i].ReqCount)
		}
	}
}
