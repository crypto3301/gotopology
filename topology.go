package main

import (
	"fmt"
	"strings"
)

type TopologyConfig struct {
	Nodes []int    `json:"nodes"`
	Edges [][2]int `json:"edges"`
}

func ValidateTopology(nodes []int, edges [][2]int) error {
	n := len(nodes)
	if n == 0 {
		return fmt.Errorf("no nodes defined")
	}
	if n > 50 {
		return fmt.Errorf("too many nodes (%d), maximum is 50", n)
	}

	nodeSet := make(map[int]bool)
	for _, id := range nodes {
		nodeSet[id] = true
	}

	adj := make(map[int][]int)
	for _, id := range nodes {
		adj[id] = []int{}
	}
	for _, e := range edges {
		u, v := e[0], e[1]
		if !nodeSet[u] {
			return fmt.Errorf("edge (%d,%d): node %d not in nodes list", u, v, u)
		}
		if !nodeSet[v] {
			return fmt.Errorf("edge (%d,%d): node %d not in nodes list", u, v, v)
		}
		if u == v {
			return fmt.Errorf("self-loop on node %d is not allowed", u)
		}
		adj[u] = append(adj[u], v)
		adj[v] = append(adj[v], u)
	}

	if !isConnected(nodes, adj) {
		return fmt.Errorf("graph is not connected")
	}

	bridges := findBridges(nodes, adj)
	if len(bridges) > 0 {
		parts := make([]string, len(bridges))
		for i, b := range bridges {
			parts[i] = fmt.Sprintf("(%d-%d)", b[0], b[1])
		}
		return fmt.Errorf("graph has %d bridge(s): %s — add redundant edges to fix",
			len(bridges), strings.Join(parts, ", "))
	}

	return nil
}

func isConnected(nodes []int, adj map[int][]int) bool {
	if len(nodes) == 0 {
		return true
	}
	visited := make(map[int]bool)
	queue := []int{nodes[0]}
	visited[nodes[0]] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range adj[cur] {
			if !visited[nb] {
				visited[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return len(visited) == len(nodes)
}

type bridgeFinder struct {
	adj     map[int][]int
	visited map[int]bool
	disc    map[int]int
	low     map[int]int
	timer   int
	bridges [][2]int
}

func findBridges(nodes []int, adj map[int][]int) [][2]int {
	bf := &bridgeFinder{
		adj:     adj,
		visited: make(map[int]bool),
		disc:    make(map[int]int),
		low:     make(map[int]int),
	}
	for _, n := range nodes {
		if !bf.visited[n] {
			bf.dfs(n, -1)
		}
	}
	return bf.bridges
}

func (bf *bridgeFinder) dfs(u, parent int) {
	bf.visited[u] = true
	bf.disc[u] = bf.timer
	bf.low[u] = bf.timer
	bf.timer++

	for _, v := range bf.adj[u] {
		if !bf.visited[v] {
			bf.dfs(v, u)
			if bf.low[v] < bf.low[u] {
				bf.low[u] = bf.low[v]
			}
			if bf.low[v] > bf.disc[u] {
				bf.bridges = append(bf.bridges, [2]int{u, v})
			}
		} else if v != parent {
			if bf.disc[v] < bf.low[u] {
				bf.low[u] = bf.disc[v]
			}
		}
	}
}
