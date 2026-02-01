package cycles

// Cycle represents a detected circular dependency.
type Cycle struct {
	Nodes    []string    `json:"nodes"`
	Edges    []CycleEdge `json:"edges"`
	Severity string      `json:"severity"` // high/medium/low based on size
	Size     int         `json:"size"`
}

// CycleEdge represents an edge within a cycle.
type CycleEdge struct {
	From        string  `json:"from"`
	To          string  `json:"to"`
	Strength    int     `json:"strength"`    // ImportCount or ref count
	BreakCost   float64 `json:"breakCost"`   // 0-1, lower = easier to break
	Recommended bool    `json:"recommended"` // suggested edge to break
}

// EdgeMeta provides metadata about an edge for break-cost calculation.
type EdgeMeta struct {
	Strength int // import count or reference count
}

// DetectOptions configures cycle detection.
type DetectOptions struct {
	MaxCycles int // maximum cycles to return (0 = unlimited)
}

// DetectResult contains all detected cycles.
type DetectResult struct {
	Cycles      []Cycle `json:"cycles"`
	TotalCycles int     `json:"totalCycles"`
	Granularity string  `json:"granularity"`
}

// Detector finds circular dependencies using Tarjan's SCC algorithm.
type Detector struct{}

// NewDetector creates a new cycle detector.
func NewDetector() *Detector {
	return &Detector{}
}

// Detect finds all circular dependencies in the given dependency graph.
// Uses Tarjan's SCC algorithm to find strongly connected components.
func (d *Detector) Detect(nodes []string, adjacency map[string][]string, edgeMeta map[[2]string]EdgeMeta, opts DetectOptions) *DetectResult {
	if len(nodes) == 0 {
		return &DetectResult{}
	}

	// Tarjan's SCC algorithm state
	type nodeState struct {
		index   int
		lowlink int
		onStack bool
	}

	state := make(map[string]*nodeState)
	var stack []string
	var sccs [][]string
	index := 0

	var strongConnect func(v string)
	strongConnect = func(v string) {
		state[v] = &nodeState{
			index:   index,
			lowlink: index,
			onStack: true,
		}
		index++
		stack = append(stack, v)

		for _, w := range adjacency[v] {
			if s, ok := state[w]; !ok {
				// w has not yet been visited
				strongConnect(w)
				if state[w].lowlink < state[v].lowlink {
					state[v].lowlink = state[w].lowlink
				}
			} else if s.onStack {
				// w is on the stack → part of current SCC
				if s.index < state[v].lowlink {
					state[v].lowlink = s.index
				}
			}
		}

		// If v is a root node, pop the SCC
		if state[v].lowlink == state[v].index {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				state[w].onStack = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			// Only keep SCCs with size > 1 (actual cycles)
			if len(scc) > 1 {
				sccs = append(sccs, scc)
			}
		}
	}

	// Run Tarjan's on all nodes
	for _, node := range nodes {
		if _, visited := state[node]; !visited {
			strongConnect(node)
		}
	}

	// Convert SCCs to Cycle structs
	cycles := make([]Cycle, 0, len(sccs))
	for _, scc := range sccs {
		cycle := d.buildCycle(scc, adjacency, edgeMeta)
		cycles = append(cycles, cycle)
	}

	totalCycles := len(cycles)

	// Apply max limit
	if opts.MaxCycles > 0 && len(cycles) > opts.MaxCycles {
		cycles = cycles[:opts.MaxCycles]
	}

	return &DetectResult{
		Cycles:      cycles,
		TotalCycles: totalCycles,
	}
}

// buildCycle constructs a Cycle from an SCC, identifying edges and the recommended break point.
func (d *Detector) buildCycle(scc []string, adjacency map[string][]string, edgeMeta map[[2]string]EdgeMeta) Cycle {
	// Build set for quick lookup
	sccSet := make(map[string]bool, len(scc))
	for _, n := range scc {
		sccSet[n] = true
	}

	// Find all edges within the SCC
	var edges []CycleEdge
	minStrength := -1
	minStrengthIdx := -1

	for _, from := range scc {
		for _, to := range adjacency[from] {
			if !sccSet[to] {
				continue
			}
			key := [2]string{from, to}
			strength := 1
			if meta, ok := edgeMeta[key]; ok {
				strength = meta.Strength
				if strength <= 0 {
					strength = 1
				}
			}

			edges = append(edges, CycleEdge{
				From:     from,
				To:       to,
				Strength: strength,
			})

			if minStrength < 0 || strength < minStrength {
				minStrength = strength
				minStrengthIdx = len(edges) - 1
			}
		}
	}

	// Mark the weakest edge as recommended to break
	if minStrengthIdx >= 0 {
		edges[minStrengthIdx].Recommended = true
	}

	// Calculate break costs
	maxStrength := 0
	for _, e := range edges {
		if e.Strength > maxStrength {
			maxStrength = e.Strength
		}
	}
	if maxStrength > 0 {
		for i := range edges {
			edges[i].BreakCost = float64(edges[i].Strength) / float64(maxStrength)
		}
	}

	size := len(scc)
	return Cycle{
		Nodes:    scc,
		Edges:    edges,
		Severity: cycleSeverity(size),
		Size:     size,
	}
}

// cycleSeverity returns severity based on cycle size.
func cycleSeverity(size int) string {
	if size >= 5 {
		return "high"
	}
	if size >= 3 {
		return "medium"
	}
	return "low"
}
