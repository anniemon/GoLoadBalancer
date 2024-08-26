package main

import (
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jonboulle/clockwork"
)

// TODO: refactor to separate NodeManager file
type NodeManager interface {
	GetNextNode() *Node
	isRateLimitExceeded(node *Node, bodyLen int) bool
	ResetLimits()
	CheckHealth()
}

type SafeNodeManager struct {
	nodes []*Node
	next  int32
	clock clockwork.Clock
}

type LoadBalancer struct {
	manager NodeManager
}

func NewSafeNodeManager(nodes []*Node, clock clockwork.Clock) *SafeNodeManager {
	return &SafeNodeManager{
			nodes: nodes,
			next:  0,
			clock: clock,
	}
}

func (m *SafeNodeManager) GetNextNode() *Node {
	idx := atomic.AddInt32(&m.next, 1) % int32(len(m.nodes))
	node := m.nodes[idx]

	if node.Healthy == 1 {
			return node
	}

	log.Printf("Skipping unhealthy node %d (%s)", node.ID, node.URL)
	return nil
}

func (m *SafeNodeManager) isRateLimitExceeded(node *Node, bodyLen int) bool {
	if atomic.LoadUint32(&node.ReqCount) >= node.ReqLimit ||
		atomic.LoadUint64(&node.BodyCount)+uint64(bodyLen) > node.BodyLimit {
			log.Printf("Rate limit hit for node %d (%s) - RPM: %d/%d, BodyLimit: %d, RequestBody: %d\n",
			node.ID, node.URL, node.ReqCount, node.ReqLimit, node.BodyLimit, bodyLen)
			return true
	}

	atomic.AddUint32(&node.ReqCount, 1)
	atomic.AddUint64(&node.BodyCount, uint64(bodyLen))
	
	return false
}

func (m *SafeNodeManager) ResetLimits() {
	for _, node := range m.nodes {
			atomic.StoreUint32(&node.ReqCount, 0)
			atomic.StoreUint64(&node.BodyCount, 0)
	}
}

func (m *SafeNodeManager) CheckHealth() {
	for _, node := range m.nodes {
			go func(n *Node) {
					resp, err := http.Get(n.URL + "/health")

					if err != nil || resp.StatusCode != http.StatusOK {
							atomic.StoreUint32(&n.Healthy, 0)
							log.Printf("Node %d (%s) is unhealthy\n", n.ID, n.URL)
					} else {
							atomic.StoreUint32(&n.Healthy, 1)
							log.Printf("Node %d (%s) is healthy\n", n.ID, n.URL)
					}

					if resp != nil {
							resp.Body.Close()
					}
			}(node)
	}
}

func NewLoadBalancer(manager NodeManager) *LoadBalancer {
	return &LoadBalancer{
			manager: manager,
	}
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bodyLen := int(r.ContentLength)
  nodeLen := len(lb.manager.(*SafeNodeManager).nodes)

	var node *Node
	for i := 0; i < nodeLen; i++ {
			node = lb.manager.GetNextNode()
			if node == nil {
					continue
			}

			if lb.manager.isRateLimitExceeded(node, bodyLen) {
					continue
			}

			log.Printf("Forwarding request to node %d (%s)\n - RPM: %d/%d, BPM: %d/%d\n",
			node.ID, node.URL, node.ReqCount, node.ReqLimit, node.BodyCount, node.BodyLimit)

			node.ReverseProxy.ServeHTTP(w, r)
			return;
	}

	http.Error(w, "No available node", http.StatusServiceUnavailable)
}

func (m *SafeNodeManager) StartPeriodicTasks() {
	m.clock.AfterFunc(1*time.Minute, m.ResetLimits)
	m.clock.AfterFunc(30*time.Second, m.CheckHealth)
}
