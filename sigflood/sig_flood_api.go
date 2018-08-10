package main

import (
	"math/rand"
	"sort"
)

var METHODS = map[string]int{"normal":0, "binning":1, "puzzle":2}

/* Paths ordered from destination->source. ASes separated by commas. */
var REAL_PATHS = []string{"A,B,C,D", "F,G,D", "F,I,D", "E,F,G,D", "H,F,G,D", "E,F,I,D", "H,F,I,D", "J,K,L,D"}
var FAKE_PATHS = []string{"E,F,G,D", "H,F,G,D", "E,F,I,D", "H,F,I,D", "M,F,G,D", "N,F,G,D", "O,F,G,D", "M,F,I,D", "N,F,I,D", "O,F,I,D"}

type kv struct {
	K   string
	V int
}

func FindAttacker(paths [][]string, num_attackers int) []kv {
	/* Create dictionary of AS : #AppearancesInPaths */
	all_nodes := make(map[string]int)
	for _, path := range paths {
		for _, node := range path {
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
