package world

// Walking components remain symmetric. Directed movement connects components
// without merging them: jumping down must never establish an uphill route.
func componentReachability(grid *Grid, comps [][]int) map[int][]int {
	links := map[int][]int{}
	for y := 0; y < grid.Height; y++ {
		for x := 0; x < grid.Width; x++ {
			from := comps[y][x]
			if from == 0 {
				continue
			}
			for _, dir := range stepDirs {
				step, ok := grid.Movement(x, y, dir, nil)
				if !ok || step == dir {
					continue
				}
				to := comps[y+step.DY][x+step.DX]
				if to != 0 && to != from {
					links[from] = append(links[from], to)
				}
			}
		}
	}
	out := map[int][]int{}
	for start := range links {
		queue := []int{start}
		seen := map[int]bool{start: true}
		for i := 0; i < len(queue); i++ {
			for _, next := range links[queue[i]] {
				if !seen[next] {
					seen[next] = true
					queue = append(queue, next)
				}
			}
		}
		out[start] = queue
	}
	return out
}

func (g *Graph) expandComponents(id uint8, in []int) []int {
	var out []int
	seen := map[int]bool{}
	for _, c := range in {
		reachable := g.reachable[id][c]
		if len(reachable) == 0 {
			reachable = []int{c}
		}
		for _, r := range reachable {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}
