package inspect

import (
	"runtime"
	"sort"
	"strconv"
	"strings"

	"ergo.services/ergo/lib"
)

// goroutineDumpSize estimates the dump size from the goroutine count. Every
// runtime.Stack of all goroutines stops the world, so an undersized buffer costs
// another pause per retry.
func goroutineDumpSize() int {
	const (
		perGoroutine = 4096
		minSize      = 1 << 20
		maxSize      = 256 << 20
	)

	size := int(lib.ReadRuntimeMetrics().Goroutines) * perGoroutine
	if size < minSize {
		return minSize
	}
	if size > maxSize {
		return maxSize
	}
	return size
}

func captureGoroutines(req RequestGetGoroutines) ResponseGetGoroutines {
	buf := make([]byte, goroutineDumpSize())
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, len(buf)*2)
	}

	blocks := strings.Split(string(buf), "\n\n")

	stackFilter := strings.ToLower(req.Stack)
	stateFilter := strings.ToLower(req.State)

	type parsed struct {
		id      int
		state   string
		waitSec int64
		frames  string
		top     string
		bottom  string
		full    string
	}

	var matched []parsed
	total := 0

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if strings.HasPrefix(block, "goroutine ") == false {
			continue
		}
		total++

		id, state, waitSec := parseHeader(block)

		if stateFilter != "" && strings.ToLower(state) != stateFilter {
			continue
		}
		if req.MinWait > 0 && waitSec < req.MinWait {
			continue
		}
		if stackFilter != "" && strings.Contains(strings.ToLower(block), stackFilter) == false {
			continue
		}

		funcs := parseFuncLines(block)
		top := ""
		bottom := ""
		if len(funcs) > 0 {
			top = funcs[0]
			bottom = funcs[len(funcs)-1]
		}

		matched = append(matched, parsed{
			id:      id,
			state:   state,
			waitSec: waitSec,
			frames:  state + "|" + strings.Join(funcs, "|"),
			top:     top,
			bottom:  bottom,
			full:    block,
		})
	}

	// group by identical stack
	groupMap := make(map[string]*GoroutineGroup)
	var order []string

	for _, p := range matched {
		g, ok := groupMap[p.frames]
		if ok == false {
			g = &GoroutineGroup{State: p.state, WaitSec: p.waitSec, Current: p.top, Origin: p.bottom, Stack: p.full}
			groupMap[p.frames] = g
			order = append(order, p.frames)
		}
		g.Count++
		g.IDs = append(g.IDs, p.id)
	}

	groups := make([]GoroutineGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, *groupMap[key])
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Count > groups[j].Count
	})

	return ResponseGetGoroutines{
		Groups:   groups,
		Total:    total,
		Filtered: len(matched),
	}
}

func parseHeader(block string) (id int, state string, waitSec int64) {
	lines := strings.SplitN(block, "\n", 2)
	header := lines[0]

	rest := header[len("goroutine "):]
	spaceIdx := strings.IndexByte(rest, ' ')
	if spaceIdx < 0 {
		return
	}
	id, _ = strconv.Atoi(rest[:spaceIdx])

	open := strings.IndexByte(rest, '[')
	close := strings.IndexByte(rest, ']')
	if open < 0 || close <= open {
		return
	}

	stateStr := rest[open+1 : close]
	parts := strings.SplitN(stateStr, ",", 2)
	state = strings.TrimSpace(parts[0])

	if len(parts) > 1 {
		waitSec = parseWaitDuration(strings.TrimSpace(parts[1]))
	}
	return
}

func parseWaitDuration(s string) int64 {
	// "5 minutes", "847 minutes", "2 hours"
	parts := strings.Fields(s)
	if len(parts) < 2 {
		return 0
	}
	n, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}
	unit := strings.TrimSuffix(parts[1], "s") // "minute" from "minutes"
	switch unit {
	case "minute":
		return n * 60
	case "hour":
		return n * 3600
	case "second":
		return n
	}
	return 0
}

func parseFuncLines(block string) []string {
	lines := strings.Split(block, "\n")
	var funcs []string
	for i := 1; i < len(lines); i++ {
		if len(lines[i]) > 0 && lines[i][0] != '\t' {
			f := strings.TrimSpace(lines[i])
			if f != "" {
				funcs = append(funcs, f)
			}
		}
	}
	return funcs
}
