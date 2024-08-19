package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
)

type LoadBalancer struct {
	Nodes []*Node
	Next  int
	Mutex sync.Mutex
	Clock clockwork.Clock
}

func NewLoadBalancer(clock clockwork.Clock) *LoadBalancer {
	return &LoadBalancer{
		Nodes: []*Node{},
		Next:  0,
		Mutex: sync.Mutex{},
		Clock: clock,
	}
}

func (lb *LoadBalancer) AddNode(node *Node) {
	lb.Nodes = append(lb.Nodes, node)
}

func (lb *LoadBalancer) GetNextNode() *Node {
	lb.Mutex.Lock()
	defer lb.Mutex.Unlock()

	node := lb.Nodes[lb.Next]
	lb.Next = (lb.Next + 1) % len(lb.Nodes)

	if node.Healthy {
		return node
	}

	log.Printf("Skipping unhealthy node %d (%s)",
			node.ID, node.URL)
	return nil
}

func isRateLimitExceeded(node *Node, bodyLen int) (bool) {
	if node.ReqCount >= node.ReqLimit || node.BodyCount+bodyLen > node.BodyLimit {
		return true
	}
	return false
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bodyLen := int(r.ContentLength)

	for i := 0; i < len(lb.Nodes); i++ {
		node := lb.GetNextNode()

		if node == nil {
			continue
		}

		node.Mutex.Lock()
		defer node.Mutex.Unlock()

		if isRateLimitExceeded(node, bodyLen) {
			log.Printf("Rate limit hit for node %d (%s) - RPM: %d/%d, BodyLimit: %d, RequestBody: %d\n",
			node.ID, node.URL, node.ReqCount, node.ReqLimit, node.BodyLimit, bodyLen)

			continue
		}

		node.ReqCount++
		node.BodyCount += bodyLen

		log.Printf("Forwarding request to node %d (%s) - RPM: %d/%d, BPM: %d/%d\n",
		node.ID, node.URL, node.ReqCount, node.ReqLimit, node.BodyCount, node.BodyLimit)

		node.ReverseProxy.ServeHTTP(w, r)
		return
	}

	log.Println("No available node")
	http.Error(w, "No available node", http.StatusServiceUnavailable)
}

func (lb *LoadBalancer) StartPeriodicTasks() {
	lb.Clock.AfterFunc(1*time.Minute, lb.resetLimits)
	lb.Clock.AfterFunc(30*time.Second, lb.checkHealth)
}

func (lb *LoadBalancer) resetLimits() {
	for _, node := range lb.Nodes {
		node.Mutex.Lock()
		node.ReqCount = 0
		node.BodyCount = 0
		node.Mutex.Unlock()
	}
	lb.Clock.AfterFunc(1*time.Minute, lb.resetLimits)
}

func (lb *LoadBalancer) checkHealth() {
	for _, node := range lb.Nodes {
		go func(n *Node) {
			resp, err := http.Get(n.URL + "/health")
			n.Mutex.Lock()

			if err != nil || resp.StatusCode != http.StatusOK {
				n.Healthy = false
				log.Printf("Node %d (%s) is unhealthy\n", node.ID, node.URL)

			} else {
				n.Healthy = true
				log.Printf("Node %d (%s) is healthy\n", node.ID, node.URL)
			}
			n.Mutex.Unlock()
			if resp != nil {
				resp.Body.Close()
			}
		}(node)
	}
	lb.Clock.AfterFunc(30*time.Second, lb.checkHealth)
}
