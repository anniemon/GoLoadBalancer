package main

import (
	"net/http/httputil"
	"net/url"
	"sync"
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
	return node
}

func (n *Node) ResetLimits() {
	n.Mutex.Lock()
	defer n.Mutex.Unlock()
	n.ReqCount = 0
	n.BodyCount = 0
}
