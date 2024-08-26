package main

import (
	"net/http/httputil"
	"net/url"
)

type NodeParams struct {
	ID        int
	URL       string
	ReqLimit  uint32
	BodyLimit uint64
}

type Node struct {
	ID         int
	URL        string
	ReqLimit   uint32
	BodyLimit  uint64
	ReqCount   uint32
	BodyCount  uint64
	Healthy    uint32
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
		Healthy:   1,
		ReverseProxy: rp,
	}
	return node
}
