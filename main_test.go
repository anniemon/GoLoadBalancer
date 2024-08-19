package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
)

var (
	client    = &http.Client{}
	lbServer  *httptest.Server
	lb       = &LoadBalancer{}
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
		lb.AddNode(NewNode(nodeParams))
	}
	lbServer = httptest.NewServer(lb)
	time.Sleep(1 * time.Second)
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

	for i := 0; i <= len(lb.Nodes) + 1; i++ {
		node := lb.Nodes[next]

		if node.ID != next+1 {
			t.Errorf("Round robin not working correctly, expected %d, got %d", next+1, node.ID)
		}
		next = (next + 1) % len(lb.Nodes)
	}
}

// Should hit rate limit by requests per minute for each node
func testRateLimitExceededByRPM(t *testing.T) {
	resetRateLimits(lb)
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

	for _, node := range lb.Nodes {
		node.Healthy = false
	}

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
	c := clockwork.NewFakeClock()

	lb2 := NewLoadBalancer(c)
	node := NewNode(nodeParams[0])
	lb2.AddNode(node)

	node.ReqCount = 10
	node.BodyCount = 100

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		lb2.StartPeriodicTasks()
		c.Advance(61 * time.Second)
		wg.Done()
	}()
	c.BlockUntil(1)

	wg.Wait()

	if node.ReqCount != 0 || node.BodyCount != 0 {
		t.Errorf("Expected counters to be reset to 0, got ReqCount=%d, BodyCount=%d", node.ReqCount, node.BodyCount)
	}
}
