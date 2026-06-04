package shoproute

// Order re-sequences an itinerary into a sensible visiting order starting from
// the given zone, using a nearest-neighbour walk over a zone-connectivity
// graph: from the current zone, go to the nearest unvisited stop (fewest zone
// hops), repeat. It's a heuristic, not a guaranteed-shortest tour.
//
// Important: the graph models zone-to-zone connections only. Real EQ travel
// also uses ports, Gate, boats, and bind points, none of which are represented
// here — so the result is a "reasonable order," not an optimal route. The
// Plane of Knowledge book hub *is* in the graph, so routes naturally benefit
// from it. Zones unreachable from the start (or each other) are appended last
// in deterministic order.
//
// adj is an undirected adjacency map of zone short_name → neighbouring zone
// short_names. start is a zone short_name (need not be one of the stops). With
// an empty start or fewer than two stops, the input order is returned as-is.
func Order(stops []Stop, start string, adj map[string][]string) []Stop {
	if start == "" || len(stops) < 2 {
		return stops
	}

	byZone := make(map[string]Stop, len(stops))
	remaining := make(map[string]bool, len(stops))
	for _, s := range stops {
		byZone[s.Zone] = s
		remaining[s.Zone] = true
	}

	ordered := make([]Stop, 0, len(stops))
	current := start
	for len(remaining) > 0 {
		dist := bfsDistances(current, adj)
		best, bestDist := "", 0
		for z := range remaining {
			d, ok := dist[z]
			if !ok {
				d = distUnreachable
			}
			if best == "" || d < bestDist || (d == bestDist && z < best) {
				best, bestDist = z, d
			}
		}
		ordered = append(ordered, byZone[best])
		delete(remaining, best)
		current = best
	}
	return ordered
}

// LinkHub returns a copy of adj with undirected edges added from hub to each
// destination zone. It models cheap teleport access from a bind/port hub (the
// Nexus): once linked, every portable zone sits one hop from the hub, so the
// solver treats it as easy to reach. It's a star, not a mesh — destinations
// connect to each other only through the hub (two hops), which mirrors "gate to
// bind, port out again" without collapsing every zone-to-zone distance.
//
// Existing edges are preserved; self-links and duplicates are skipped.
func LinkHub(adj map[string][]string, hub string, dests []string) map[string][]string {
	out := make(map[string][]string, len(adj)+1)
	seen := make(map[string]map[string]bool, len(adj)+1)
	for z, ns := range adj {
		out[z] = append([]string(nil), ns...)
		seen[z] = make(map[string]bool, len(ns))
		for _, n := range ns {
			seen[z][n] = true
		}
	}
	add := func(a, b string) {
		if a == b {
			return
		}
		if seen[a] == nil {
			seen[a] = map[string]bool{}
		}
		if !seen[a][b] {
			seen[a][b] = true
			out[a] = append(out[a], b)
		}
	}
	for _, d := range dests {
		add(hub, d)
		add(d, hub)
	}
	return out
}

// Distances returns hop-distance from start to every zone reachable over the
// adjacency graph (start maps to 0; absent zones are unreachable). The solver
// uses it to prefer nearer sources when several zones tie on coverage.
func Distances(start string, adj map[string][]string) map[string]int {
	return bfsDistances(start, adj)
}

// bfsDistances returns the hop distance from start to every reachable zone.
// start maps to 0; zones not present in the result are unreachable.
func bfsDistances(start string, adj map[string][]string) map[string]int {
	dist := map[string]int{start: 0}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if _, seen := dist[next]; !seen {
				dist[next] = dist[cur] + 1
				queue = append(queue, next)
			}
		}
	}
	return dist
}
