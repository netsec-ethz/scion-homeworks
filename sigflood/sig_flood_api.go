package main

import (
	"math/rand"
	"sort"
	"strings"
)

const (
	TIMESTAMP_SIZE = 16
	PAYLOAD_SIZE = 48
)

var METHODS = map[string]int{"normal":0, "binning":1, "puzzle":2}

/* Paths ordered from destination->source. ASes separated by commas. */
var REAL_PATHS = []string{"D,C,B,A", "D,G,F", "D,I,F", "D,G,F,E", "D,G,F,H", "D,I,F,E", "D,I,F,H", "D,L,K,J"}
var FAKE_PATHS = []string{"D,G,F,E", "D,G,F,H", "D,I,F,E", "D,I,F,H", "D,G,F,M", "D,G,F,N", "D,G,F,O", "D,I,F,M", "D,I,F,N", "D,I,F,O"}

type kv struct {
	K   string
	V int
}

func FindAttacker(paths []string, num_attackers int) []kv {
	/* Create dictionary of AS : #AppearancesInPaths */
	all_nodes := make(map[string]int)
	for _, path := range paths {
		/* Destination is the service itself so do not include. */
		for _, node := range strings.Split(path, ",")[1:] {
			_, in := all_nodes[node]
		if !in {
			all_nodes[node] = 0
		}
		all_nodes[node] += 1
		}
	}

	/* Get the top num_attacker ASes that appeared */
	var nodes_list []kv
	for k,v := range all_nodes {
		nodes_list = append(nodes_list, kv{k,v})
	}

	sort.Slice(nodes_list, func(i, j int) bool {
		return nodes_list[i].V > nodes_list[j].V
	})

	return nodes_list[:num_attackers]
}

func GetRandomPath(seed rand.Source, realUser bool) string {
	if realUser {
		return REAL_PATHS[rand.New(seed).Intn(len(REAL_PATHS))]
	} else {
		return FAKE_PATHS[rand.New(seed).Intn(len(FAKE_PATHS))]
	}
}
