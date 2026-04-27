package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
)

func main() {
	configPath := flag.String("config", "topology.json", "Path to topology JSON file")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	data, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Cannot read config file: %v", err)
	}

	var cfg TopologyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Cannot parse config: %v", err)
	}

	net, err := NewNetwork(cfg)
	if err != nil {
		log.Fatalf("Invalid topology: %v", err)
	}

	net.Start()

	srv := NewServer(net, *addr)
	log.Printf("DSR Simulation → http://localhost%s", *addr)
	log.Fatal(srv.ListenAndServe())
}
