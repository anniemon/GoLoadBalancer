package main

import (
	"bytes"
	"fmt"
	"io"
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
	servers   []*http.Server
	nodeManager *SafeNodeManager
	nodeParams     = []NodeParams{
		{ID: 1, URL: "http://localhost:8081", ReqLimit: 2, BodyLimit: 76},
		{ID: 2, URL: "http://localhost:8082", ReqLimit: 3, BodyLimit: 2 * 1024 * 1024},
		{ID: 3, URL: "http://localhost:8083", ReqLimit: 5, BodyLimit: 1 * 1024 * 1024},
	}
)

func startTestServer(id int) *http.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("node: %d", id)))
	})
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", 8080 + id),
		Handler: handler,
	}
	go server.ListenAndServe()
	return server
}

func setup() {
	nodes := make([]*Node, len(nodeParams))
	for i, nodeParams := range nodeParams {
		startTestServer(nodeParams.ID)
		nodes[i] = NewNode(nodeParams)
	}

	nodeManager = NewSafeNodeManager(nodes, clockwork.NewFakeClock())
	nodeManager.StartPeriodicTasks()
	lb := NewLoadBalancer(nodeManager)

	lbServer = httptest.NewServer(lb)
	time.Sleep(1 * time.Second)
}


func teardown() {
	lbServer.Close()
	for _, server := range servers {
		server.Close()
	}
}

func resetRateLimits(m *SafeNodeManager) {
	m.ResetLimits()
}

func resetHealthChecks(m *SafeNodeManager) {
	for _, node := range m.nodes {
		node.Healthy = 1
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
	t.Run("RateLimitExceededByBPM", testRateLimitExceededByBPM)
}

// Should work by round robin
func testRoundRobin(t *testing.T) {
	next := 0

	for i := 0; i <= len(nodeManager.nodes) + 1; i++ {
		node := nodeManager.nodes[next]

		if node.ID != next + 1 {
			t.Errorf("Round robin not working correctly, expected %d, got %d", next+1, node.ID)
		}
		next = (next + 1) % len(nodeManager.nodes)
	}
}

// Should hit rate limit by requests per minute for each node
func testRateLimitExceededByRPM(t *testing.T) {
	resetRateLimits(nodeManager)
	nodes := nodeManager.nodes

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

	if nodes[0].ReqCount <  nodes[0].ReqLimit {
		t.Errorf("Rate Limit should be hit; RPM %d/%d", nodes[0].ReqCount, nodes[0].ReqLimit)
	}
	t.Logf("Node2 ReqCount:%d, ReqLimit:%d", nodes[1].ReqCount, nodes[1].ReqLimit)
	t.Logf("Node3 ReqCount:%d, ReqLimit:%d", nodes[2].ReqCount, nodes[2].ReqLimit)
}

// Should hit rate limit by request body size per minute for each node
func testRateLimitExceededByBPM(t *testing.T) {
	resetRateLimits(nodeManager)
	resetHealthChecks(nodeManager)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Post(lbServer.URL, "text/plain", bytes.NewBufferString("this should exceed the body limit!! this sentence is under 76 bytes"))
			if err != nil {
				t.Errorf("Failed to send request: %v", err)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()

	resp, err := client.Post(lbServer.URL, "text/plain", bytes.NewBufferString("this should skip the node1"))
	if err != nil {
		t.Errorf("Failed to send request: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Failed to read response body: %v", err)
	}

	expected := "node: 2"
	if string(body) != "node: 2" {
		t.Errorf("Expected response body to be %s, got %s", expected, body)
	}
}

// Should return 503 when all nodes are unhealthy
func testAllNodesUnhealthy(t *testing.T) {
	resetRateLimits(nodeManager)

	for _, node := range nodeManager.nodes {
		node.Healthy = 0
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
	resetHealthChecks(nodeManager)

	for _, node := range nodeManager.nodes {
		node.ReqCount = node.ReqLimit
	}

	resp, err := client.Get(lbServer.URL)
	if err != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}
	resp.Body.Close()
}

// Should reset RPM and BPM for each node every minute
func testRateLimitReset(t *testing.T) {
	c := clockwork.NewFakeClock()

	nodeManager := NewSafeNodeManager(nodeManager.nodes, c)
	node := nodeManager.nodes[0]

	node.ReqCount = 10
	node.BodyCount = 100

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		nodeManager.StartPeriodicTasks()
		c.Advance(61 * time.Second)
		wg.Done()
	}()
	c.BlockUntil(1)

	wg.Wait()

	if node.ReqCount != 0 || node.BodyCount != 0 {
		t.Errorf("Expected counters to be reset to 0, got ReqCount=%d, BodyCount=%d", node.ReqCount, node.BodyCount)
	}
}
