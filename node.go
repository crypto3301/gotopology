package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type Node struct {
	id        int
	neighbors []int
	net       *Network
	inbox     chan Message

	routeCache  map[int][]int
	seenRREQ    map[string]bool
	mu          sync.Mutex
	rreqCounter atomic.Uint64
}

func NewNode(id int, neighbors []int, net *Network) *Node {
	return &Node{
		id:         id,
		neighbors:  neighbors,
		net:        net,
		inbox:      make(chan Message, 200),
		routeCache: make(map[int][]int),
		seenRREQ:   make(map[string]bool),
	}
}

func (n *Node) Run() {
	for msg := range n.inbox {
		switch msg.Type {
		case MsgRREQ:
			n.handleRREQ(msg)
		case MsgRREP:
			n.handleRREP(msg)
		}
	}
}

func (n *Node) reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.routeCache = make(map[int][]int)
	n.seenRREQ = make(map[string]bool)
}

func (n *Node) InitiateRoute(dest int) {
	n.mu.Lock()
	if route, ok := n.routeCache[dest]; ok {
		n.mu.Unlock()
		n.net.Emit(Event{
			Type:        EvCacheHit,
			Node:        n.id,
			Source:      n.id,
			Destination: dest,
			Route:       copySlice(route),
			Message:     fmt.Sprintf("Node %d: cache hit → %d via [%s]", n.id, dest, routeStr(route)),
		})
		return
	}

	seq := n.rreqCounter.Add(1)
	rreqID := fmt.Sprintf("%d:%d", n.id, seq)
	n.seenRREQ[rreqID] = true
	n.mu.Unlock()

	n.net.Emit(Event{
		Type:    EvLog,
		Message: fmt.Sprintf("Node %d: initiating route discovery to %d (RREQ-ID: %s)", n.id, dest, rreqID),
	})

	rreq := &RREQPacket{
		ID:          rreqID,
		Source:      n.id,
		Destination: dest,
		RouteRecord: []int{n.id},
	}

	msg := Message{Type: MsgRREQ, RREQ: rreq, From: n.id}
	for _, nb := range n.neighbors {
		n.net.Emit(Event{
			Type:        EvRREQ,
			From:        n.id,
			To:          nb,
			RouteRecord: copySlice(rreq.RouteRecord),
			RREQId:      rreqID,
			Source:      rreq.Source,
			Destination: rreq.Destination,
		})
		n.net.Send(n.id, nb, msg)
	}
}

func (n *Node) handleRREQ(msg Message) {
	rreq := msg.RREQ

	n.mu.Lock()
	if n.seenRREQ[rreq.ID] {
		n.mu.Unlock()
		n.net.Emit(Event{
			Type:    EvRREQSeen,
			Node:    n.id,
			RREQId:  rreq.ID,
			Message: fmt.Sprintf("Node %d: duplicate RREQ %s discarded", n.id, rreq.ID),
		})
		return
	}
	for _, hop := range rreq.RouteRecord {
		if hop == n.id {
			n.mu.Unlock()
			return
		}
	}
	n.seenRREQ[rreq.ID] = true
	n.mu.Unlock()

	newRecord := append(copySlice(rreq.RouteRecord), n.id)

	if n.id == rreq.Destination {
		n.net.Emit(Event{
			Type:    EvLog,
			Message: fmt.Sprintf("Node %d (destination): RREQ received, full route: [%s]", n.id, routeStr(newRecord)),
		})
		n.sendRREP(rreq.Source, newRecord)
		return
	}

	forward := &RREQPacket{
		ID:          rreq.ID,
		Source:      rreq.Source,
		Destination: rreq.Destination,
		RouteRecord: newRecord,
	}
	fwdMsg := Message{Type: MsgRREQ, RREQ: forward, From: n.id}

	inRecord := make(map[int]bool)
	for _, hop := range newRecord {
		inRecord[hop] = true
	}

	for _, nb := range n.neighbors {
		if nb == msg.From || inRecord[nb] {
			continue
		}
		n.net.Emit(Event{
			Type:        EvRREQ,
			From:        n.id,
			To:          nb,
			RouteRecord: copySlice(newRecord),
			RREQId:      rreq.ID,
			Source:      rreq.Source,
			Destination: rreq.Destination,
		})
		n.net.Send(n.id, nb, fwdMsg)
	}
}

func (n *Node) sendRREP(source int, route []int) {
	if len(route) < 2 {
		return
	}
	nextHop := route[len(route)-2]

	n.net.Emit(Event{
		Type:        EvRREP,
		From:        n.id,
		To:          nextHop,
		Route:       copySlice(route),
		Source:      source,
		Destination: n.id,
	})
	n.net.Send(n.id, nextHop, Message{
		Type: MsgRREP,
		RREP: &RREPPacket{Source: source, Destination: n.id, Route: route},
		From: n.id,
	})
}

func (n *Node) handleRREP(msg Message) {
	rrep := msg.RREP

	n.mu.Lock()
	n.routeCache[rrep.Destination] = copySlice(rrep.Route)
	n.mu.Unlock()

	n.net.Emit(Event{
		Type:    EvLog,
		Message: fmt.Sprintf("Node %d: cached route to %d → [%s]", n.id, rrep.Destination, routeStr(rrep.Route)),
	})

	if n.id == rrep.Source {
		n.net.Emit(Event{
			Type:        EvRouteFound,
			Source:      rrep.Source,
			Destination: rrep.Destination,
			Route:       copySlice(rrep.Route),
			Message:     fmt.Sprintf("Route found: [%s]", routeStr(rrep.Route)),
		})
		return
	}

	myIdx := -1
	for i, hop := range rrep.Route {
		if hop == n.id {
			myIdx = i
			break
		}
	}
	if myIdx <= 0 {
		return
	}

	nextHop := rrep.Route[myIdx-1]
	n.net.Emit(Event{
		Type:        EvRREP,
		From:        n.id,
		To:          nextHop,
		Route:       copySlice(rrep.Route),
		Source:      rrep.Source,
		Destination: rrep.Destination,
	})
	n.net.Send(n.id, nextHop, Message{
		Type: MsgRREP,
		RREP: rrep,
		From: n.id,
	})
}

func copySlice(s []int) []int {
	out := make([]int, len(s))
	copy(out, s)
	return out
}

func routeStr(route []int) string {
	parts := make([]string, len(route))
	for i, h := range route {
		parts[i] = fmt.Sprintf("%d", h)
	}
	return strings.Join(parts, "→")
}
