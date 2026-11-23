package supervisor

import (
	"slices"
	"sort"
	"strings"
)

// The principal rules of draft 9 (SUPERVISOR.md sections 1.6 and 3.3): classes
// (F4), the reader direction of E1, the privilege of a granted service (F2a),
// the budgets of H_OS-E5 (F1) and the unheld outside target of F3.

// flag reports whether a K1 field holds exactly the single value want.
func flag(sp *ServicePlan, path, want string) bool {
	v := sp.Fields[path]
	return v.Set && len(v.Strs) == 1 && v.Strs[0] == want
}

// privileges is F2a: a granted service holds no privilege beyond its
// grantee's. Each test is an allow-list, so an unstated field is refused too.
func (c *compiler) privileges(plan *Plan, p string, granted []string) {
	for _, a := range granted {
		sp, ok := plan.Services[a]
		if !ok {
			continue // reported as ErrUnknownService
		}
		refuse := func(what string) {
			c.errs.add(ErrDelegationPrivilege{Service: a, Principal: p, What: what})
		}
		if caps := sp.Fields["allow.capabilities"]; !caps.Set || len(caps.Strs) > 0 {
			refuse("capabilities")
		}
		nnp, exempt := sp.Fields["protect/nnp"], sp.Fields["protect/nnp/exempt_on"]
		if !nnp.Set || nnp.Lo != LevelRequired || !exempt.Set || !subset(exempt.Strs, []string{"darwin", "windows"}) {
			refuse("nnp")
		}
		// run_as.group: only p's own. Compile sees names, so p's own group is
		// the group of that name; the host compares ids (Host+9).
		if g := sp.Fields["run_as.group"]; g.Set && !(len(g.Strs) == 1 && g.Strs[0] == p) {
			refuse("groups")
		}
	}
}

// unheld is the warning of F3: an enabled granted service that requires an
// outside service which is not enabled cannot come up at boot.
func (c *compiler) unheld(plan *Plan, names []string, p string, granted []string) {
	for _, a := range names {
		if !slices.Contains(granted, a) || !flag(plan.Services[a], "enabled", "true") {
			continue
		}
		es := edges(plan.Services[a])
		targets := make([]string, 0, len(es))
		for t, l := range es {
			if l == LevelRequired {
				targets = append(targets, t)
			}
		}
		sort.Strings(targets)
		for _, t := range targets {
			tp, ok := plan.Services[t]
			if !ok || slices.Contains(granted, t) || flag(tp, "enabled", "true") {
				continue
			}
			// a service in two grants fails E1 and no plan is returned, so a
			// warning is never emitted twice in a plan that exists.
			plan.Warnings = append(plan.Warnings, WarnUnheldOutsideTarget{Service: a, Target: t})
		}
	}
}

// principals applies the readers' side of E1 and the budgets of H_OS-E5.
//
// A reader is in api.read, not in api.admin and without a grant block. (A
// principal with an empty grant block is covered by the outside-grant
// direction of grants.) Every non-owner-class principal states its budgets;
// a delegated one also states pids and memory.
func (c *compiler) principals(plan *Plan, names []string) {
	admin, read := plan.API["api.admin"], plan.API["api.read"]
	delegated := map[string]bool{}
	nonOwner := map[string]bool{}
	for k, g := range plan.API {
		if p, ok := strings.CutPrefix(k, "api.grant/"); ok && !slices.Contains(admin.Strs, p) {
			nonOwner[p] = true
			delegated[p] = len(g.Strs) > 0
		}
	}
	for _, r := range read.Strs {
		if slices.Contains(admin.Strs, r) {
			continue
		}
		nonOwner[r] = true
		if _, hasGrant := plan.API["api.grant/"+r]; hasGrant {
			continue
		}
		for _, s := range names {
			if u := plan.Services[s].Fields["run_as.user"]; u.Set && len(u.Strs) == 1 && u.Strs[0] == r {
				c.errs.add(ErrDelegationUid{Service: s, Principal: r, Direction: "reader"})
			}
		}
	}
	for p := range nonOwner {
		fields := readerBudgets
		if delegated[p] {
			fields = budgetFields
		}
		for _, f := range fields {
			if _, ok := plan.API["api.budget/"+p+"/"+f]; !ok {
				c.errs.add(ErrIncomplete{Path: "api.budget/" + p + "/" + f})
			}
		}
	}
}
