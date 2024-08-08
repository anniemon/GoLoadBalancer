package main

import (
	"log"
	"net/http"
	"sync"
)

type LoadBalancer struct {
	Nodes []*Node
	Next  int
	Mutex sync.Mutex
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
