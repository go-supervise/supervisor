package supervisor

import (
	"math"
	"slices"
	"sort"
	"strings"
)

var edgeNames = map[int64]string{LevelRequired: "require", LevelWaived: "want", LevelOrder: "order"}

func satAdd(xs ...int64) int64 {
	var t int64
	for _, x := range xs {
		if t > math.MaxInt64-x {
			return math.MaxInt64
		}
		t += x
	}
	return t
}

// edges returns the resolved edges of a service: target -> level.
func edges(sp *ServicePlan) map[string]int64 {
	out := map[string]int64{}
	for p, v := range sp.Fields {
		if t, ok := strings.CutPrefix(p, "edge/"); ok && v.Set {
			out[t] = v.Lo
		}
	}
	return out
}

// graph applies section 2.2 (acyclic union of all edge kinds, known targets)
// and the wait budget of section 1.6.
func (c *compiler) graph(plan *Plan, names []string) {
	adj := map[string][]string{}
	rev := map[string][]string{}
	lvl := map[[2]string]int64{}
	for _, s := range names {
		for t, l := range edges(plan.Services[s]) {
			if _, ok := plan.Services[t]; !ok {
				c.errs.add(ErrUnknownService{Service: t, Referrer: s})
				continue
			}
			adj[s] = append(adj[s], t)
			rev[t] = append(rev[t], s)
			lvl[[2]string{s, t}] = l
		}
	}
	cycles := sccs(names, adj)
	for _, scc := range cycles {
		c.errs.add(ErrCycle{Services: scc})
	}
	if len(cycles) > 0 || unresolved(plan, names) {
		return
	}
	c.waitBudget(plan, names, adj, rev, lvl)
}

// unresolved reports whether a timing field of the budget did not resolve;
// that is already reported as ErrIncomplete or ErrConflict.
func unresolved(plan *Plan, names []string) bool {
	for _, s := range names {
		for _, p := range []string{"stop.timeout", "kill.timeout", "start.timeout", "backoff.max"} {
			if _, ok := plan.Services[s].Resolved[p]; !ok {
				return true
			}
		}
	}
	return false
}

// sccs returns the strongly connected components that hold a cycle: more than
// one service, or one service with an edge to itself. Tarjan's algorithm.
func sccs(names []string, adj map[string][]string) [][]string {
	index, low := map[string]int{}, map[string]int{}
	on := map[string]bool{}
	var stack []string
	var out [][]string
	n := 0
	var visit func(v string)
	visit = func(v string) {
		index[v], low[v] = n, n
		n++
		stack = append(stack, v)
		on[v] = true
		for _, w := range adj[v] {
			if _, seen := index[w]; !seen {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if on[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			if len(comp) > 1 || slices.Contains(adj[v], v) {
				sort.Strings(comp)
				out = append(out, comp)
			}
		}
	}
	for _, v := range names {
		if _, seen := index[v]; !seen {
			visit(v)
		}
	}
	return out
}

// waitBudget is the predicate of section 1.6 (M5, N4, D6, E9): for every
// service A with a require-target, wait.timeout(A) >= W(A), with
//
//	U(B) = W(B) + SW(B) + stop(B) + kill(B) + backoff.max(B) + start(B)
//	W(A) = max over every target C of A, of any edge kind, of U(C)
//	SW(B) = max over every dependent A of B of (SW(A) + stop(A) + kill(A)) + epsilon
func (c *compiler) waitBudget(plan *Plan, names []string, adj, rev map[string][]string, lvl map[[2]string]int64) {
	res := func(s, p string) int64 { return plan.Services[s].Resolved[p] }
	memoU, memoSW := map[string]int64{}, map[string]int64{}
	var u, w, sw func(string) int64
	sw = func(b string) int64 {
		if v, ok := memoSW[b]; ok {
			return v
		}
		best := int64(0)
		for _, a := range rev[b] {
			best = max(best, satAdd(sw(a), res(a, "stop.timeout"), res(a, "kill.timeout")))
		}
		memoSW[b] = satAdd(best, Epsilon)
		return memoSW[b]
	}
	w = func(a string) int64 {
		best := int64(0)
		for _, t := range adj[a] {
			best = max(best, u(t))
		}
		return best
	}
	u = func(b string) int64 {
		if v, ok := memoU[b]; ok {
			return v
		}
		memoU[b] = satAdd(w(b), sw(b), res(b, "stop.timeout"), res(b, "kill.timeout"),
			res(b, "backoff.max"), res(b, "start.timeout"))
		return memoU[b]
	}
	for _, a := range names {
		hasReq := false
		for _, b := range adj[a] {
			if lvl[[2]string{a, b}] == LevelRequired {
				hasReq = true
			}
		}
		if !hasReq {
			continue
		}
		// E9: every service with a require-target is checked against W(A),
		// which ranges over require, want and order targets.
		need := w(a)
		if have, ok := plan.Services[a].Resolved["wait.timeout"]; ok && have < need {
			c.errs.add(ErrWaitTooShort{Service: a, Have: have, Need: need})
		}
	}
}

// grants applies section 8.2 with REVIEW-4 E1 and E2.
//
// E1: a service runs as the delegated principal p if and only if it is in
// grant(p). E2: grant closure holds under the reverse of every edge kind, so a
// service that reaches a granted one by require, want or order is itself
// granted.
func (c *compiler) grants(plan *Plan, names []string) {
	var principals []string
	for k := range plan.API {
		if p, ok := strings.CutPrefix(k, "api.grant/"); ok {
			principals = append(principals, p)
		}
	}
	sort.Strings(principals)
	admin := plan.API["api.admin"]
	for _, p := range principals {
		g := plan.API["api.grant/"+p]
		if !slices.Contains(admin.Strs, p) {
			c.errs.add(ErrConflict{Path: "api.grant/" + p, Values: []string{"grant to a principal that is not in api.admin"},
				Layers: c.siteLayerNames()})
		}
		for _, m := range g.Strs {
			sp, ok := plan.Services[m]
			if !ok {
				c.errs.add(ErrUnknownService{Service: m, Referrer: "grant " + p})
				continue
			}
			if t := sp.Fields["terminal"]; t.Set && len(t.Strs) == 1 && t.Strs[0] == "true" {
				c.errs.add(ErrFieldAuthority{Field: m + ".terminal", Authority: "grant"})
			}
		}
		for _, s := range names {
			granted := slices.Contains(g.Strs, s)
			u := plan.Services[s].Fields["run_as.user"]
			runsAsP := u.Set && len(u.Strs) == 1 && u.Strs[0] == p
			switch {
			case granted && !runsAsP:
				c.errs.add(ErrDelegationUid{Service: s, Principal: p, Direction: "granted"})
			case !granted && runsAsP:
				c.errs.add(ErrDelegationUid{Service: s, Principal: p, Direction: "outside-grant"})
			}
		}
		for _, b := range names {
			if slices.Contains(g.Strs, b) {
				continue
			}
			for a, l := range edges(plan.Services[b]) {
				if slices.Contains(g.Strs, a) {
					c.errs.add(ErrGrantNotClosed{Principal: p, Granted: a, Dependent: b, Edge: edgeNames[l]})
				}
			}
		}
	}
}
