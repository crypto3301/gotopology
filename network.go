package main

import (
	"fmt"
	"sync"
)

type MsgType int

const (
	MsgRREQ MsgType = iota
	MsgRREP
)

type Message struct {
	Type MsgType
	RREQ *RREQPacket
	RREP *RREPPacket
	From int
}

type RREQPacket struct {
	ID          string
	Source      int
	Destination int
	RouteRecord []int
}

type RREPPacket struct {
	Source      int
	Destination int
	Route       []int
}

type EventType string

const (
	EvTopology   EventType = "topology"
	EvRREQ       EventType = "rreq"
	EvRREP       EventType = "rrep"
	EvRouteFound EventType = "route_found"
	EvCacheHit   EventType = "cache_hit"
	EvRREQSeen   EventType = "rreq_seen"
	EvLog        EventType = "log"
	EvReset      EventType = "reset"
)

type Event struct {
	Type        EventType  `json:"type"`
	From        int        `json:"from"`
	To          int        `json:"to"`
	RouteRecord []int      `json:"routeRecord,omitempty"`
	Route       []int      `json:"route,omitempty"`
	RREQId      string     `json:"rreqId,omitempty"`
	Source      int        `json:"source"`
	Destination int        `json:"destination"`
	Node        int        `json:"node"`
	Message     string     `json:"message,omitempty"`
	Nodes       []NodeInfo `json:"nodes,omitempty"`
	Edges       []EdgeInfo `json:"edges,omitempty"`
}

type NodeInfo struct {
	ID int `json:"id"`
}

type EdgeInfo struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type Network struct {
	nodes  map[int]*Node
	adj    map[int][]int
	events chan Event
	mu     sync.RWMutex
}

func NewNetwork(cfg TopologyConfig) (*Network, error) {
	if err := ValidateTopology(cfg.Nodes, cfg.Edges); err != nil {
		return nil, err
	}

	net := &Network{
		nodes:  make(map[int]*Node),
		adj:    make(map[int][]int),
		events: make(chan Event, 2000),
	}

	for _, id := range cfg.Nodes {
		net.adj[id] = []int{}
	}
	for _, e := range cfg.Edges {
		u, v := e[0], e[1]
		net.adj[u] = append(net.adj[u], v)
		net.adj[v] = append(net.adj[v], u)
	}

	for _, id := range cfg.Nodes {
		net.nodes[id] = NewNode(id, net.adj[id], net)
	}

	return net, nil
}

func (net *Network) Start() {
	for _, node := range net.nodes {
		go node.Run()
	}
}

func (net *Network) Send(from, to int, msg Message) {
	net.mu.RLock()
	node, ok := net.nodes[to]
	net.mu.RUnlock()
	if !ok {
		return
	}
	for _, nb := range net.adj[from] {
		if nb == to {
			select {
			case node.inbox <- msg:
			default:
				fmt.Printf("Warning: node %d inbox full, dropping packet\n", to)
			}
			return
		}
	}
}

func (net *Network) Emit(evt Event) {
	select {
	case net.events <- evt:
	default:
	}
}

func (net *Network) Events() <-chan Event {
	return net.events
}

func (net *Network) GetTopologyEvent() Event {
	var nodeInfos []NodeInfo
	var edgeInfos []EdgeInfo
	seen := make(map[string]bool)

	for id := range net.nodes {
		nodeInfos = append(nodeInfos, NodeInfo{ID: id})
	}
	for u, neighbors := range net.adj {
		for _, v := range neighbors {
			key := fmt.Sprintf("%d-%d", minInt(u, v), maxInt(u, v))
			if !seen[key] {
				seen[key] = true
				edgeInfos = append(edgeInfos, EdgeInfo{From: u, To: v})
			}
		}
	}
	return Event{Type: EvTopology, Nodes: nodeInfos, Edges: edgeInfos}
}

func (net *Network) InitiateRoute(source, destination int) error {
	net.mu.RLock()
	srcNode, ok := net.nodes[source]
	_, dstOk := net.nodes[destination]
	net.mu.RUnlock()

	if !ok {
		return fmt.Errorf("node %d does not exist", source)
	}
	if !dstOk {
		return fmt.Errorf("node %d does not exist", destination)
	}
	if source == destination {
		return fmt.Errorf("source and destination must differ")
	}

	go srcNode.InitiateRoute(destination)
	return nil
}

func (net *Network) Reset() {
	for _, node := range net.nodes {
		node.reset()
	}
	net.Emit(Event{Type: EvReset, Message: "Network reset: all caches cleared"})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
