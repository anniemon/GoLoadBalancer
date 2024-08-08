package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

type NodeParams struct {
	ID        int
	URL       string
	ReqLimit  int
	BodyLimit int
}

type Node struct {
	ID         int
	URL        string
	ReqLimit   int
	BodyLimit  int
	ReqCount   int
	BodyCount  int
	Healthy    bool
	Mutex      sync.Mutex
	ResetTimer *time.Timer
	ReverseProxy *httputil.ReverseProxy
}

func NewNode(params NodeParams) *Node {
	url, _ := url.Parse(params.URL)
	rp := httputil.NewSingleHostReverseProxy(url)

	node := &Node{
		ID:        params.ID,
		URL:       params.URL,
		ReqLimit:  params.ReqLimit,
		BodyLimit: params.BodyLimit,
		Healthy:   true,
		ReverseProxy: rp,
	}
	node.ResetTimer = time.AfterFunc(time.Minute, node.ResetLimits)
	go node.CheckHealth()
	return node
}

func (n *Node) ResetLimits() {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()
	n.ReqCount = 0
	n.BodyCount = 0
	n.ResetTimer = time.AfterFunc(time.Minute, n.ResetLimits)
}

func (n *Node) CheckHealth() {
	for {
		time.Sleep(30 * time.Second)
		resp, err := http.Get(n.URL + "/health")

		n.Mutex.Lock()
		if err != nil || resp.StatusCode != http.StatusOK {
			n.Healthy = false
			log.Printf("Node %d (%s) is unhealthy\n", n.ID, n.URL)
		} else {
			n.Healthy = true
			log.Printf("Node %d (%s) is healthy\n", n.ID, n.URL)
		}
		n.Mutex.Unlock()
		if resp != nil {
			resp.Body.Close()
		}
	}
}
