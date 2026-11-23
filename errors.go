package supervisor

import (
	"fmt"
	"sort"
	"strings"
)

// The typed errors of SUPERVISOR.md section 1.10 that Compile can produce.
// Each is a struct so that callers use errors.As; none is ever compared by
// message.

type ErrSyntax struct{ Layer, Msg string }
type ErrNotLiteral struct{ Layer, Key string }
type ErrUnknownKey struct{ Layer, Key string }
type ErrNotALatticeValue struct{ Layer, Key, Why string }
type ErrConflict struct {
	Path   string
	Values []string
	Layers []string
}
type ErrIncomplete struct{ Path string }
type ErrNoBaseline struct{}
type ErrBaselineNotTotal struct{ Fields []string }
type ErrFieldAuthority struct{ Field, Authority string }
type ErrWideningField struct{ Service, Field, Layer string }
type ErrUnsignedRecipe struct{ Layer string }
type ErrUnsafePath struct{ Path string }
type ErrCycle struct{ Services []string }
type ErrUnknownService struct{ Service, Referrer string }
type ErrCheckTiming struct{ Service, Why string }
type ErrWaitTooShort struct {
	Service    string
	Have, Need int64 // nanoseconds
}
type ErrSecretInEnvironment struct{ Service, Key string }
type ErrRunAsInUserMode struct{ Service, RunAs, Owner string }
type ErrForkingDaemon struct{ Layer, Service string }
type ErrUntypedParam struct{ Service, Param string }
type ErrParamInShell struct{ Service, Param string }
type ErrSchemaTooNew struct{ Baseline, Running int }

// ErrDelegationUid is the two-way rule of REVIEW-4 E1: a service runs as the
// delegated principal p if and only if it is in grant(p).
type ErrDelegationUid struct {
	Service, Principal string
	// Direction is "granted-not-p" (in grant(p) but runs as another uid) or
	// "p-not-granted" (runs as p but is outside grant(p)).
	Direction string
}

// ErrGrantNotClosed is grant closure under the reverse of every edge kind
// (D2 option A, extended by REVIEW-4 E2).
type ErrGrantNotClosed struct {
	Principal, Granted, Requirer string
	Edge                         string // "require", "want" or "order"
}

func (e ErrSyntax) Error() string { return fmt.Sprintf("ErrSyntax: %s: %s", e.Layer, e.Msg) }
func (e ErrNotLiteral) Error() string {
	return fmt.Sprintf("ErrNotLiteral: %s: %s: only literals are expressions", e.Layer, e.Key)
}
func (e ErrUnknownKey) Error() string { return fmt.Sprintf("ErrUnknownKey: %s: %s", e.Layer, e.Key) }
func (e ErrNotALatticeValue) Error() string {
	return fmt.Sprintf("ErrNotALatticeValue: %s: %s: %s", e.Layer, e.Key, e.Why)
}
func (e ErrConflict) Error() string {
	return fmt.Sprintf("ErrConflict: %s: values [%s] in layers [%s]", e.Path,
		strings.Join(e.Values, ", "), strings.Join(e.Layers, ", "))
}
func (e ErrIncomplete) Error() string { return "ErrIncomplete: " + e.Path }
func (e ErrNoBaseline) Error() string {
	return "ErrNoBaseline: no site layer has an all {} block"
}
func (e ErrBaselineNotTotal) Error() string {
	return "ErrBaselineNotTotal: " + strings.Join(e.Fields, ", ")
}
func (e ErrFieldAuthority) Error() string {
	return fmt.Sprintf("ErrFieldAuthority: %s is not allowed in %s", e.Field, e.Authority)
}
func (e ErrWideningField) Error() string {
	return fmt.Sprintf("ErrWideningField: %s: %s widened by layer %s", e.Service, e.Field, e.Layer)
}
func (e ErrUnsignedRecipe) Error() string { return "ErrUnsignedRecipe: " + e.Layer }
func (e ErrUnsafePath) Error() string     { return "ErrUnsafePath: " + e.Path }
func (e ErrCycle) Error() string {
	return "ErrCycle: " + strings.Join(e.Services, ", ")
}
func (e ErrUnknownService) Error() string {
	return fmt.Sprintf("ErrUnknownService: %s (named by %s)", e.Service, e.Referrer)
}
func (e ErrCheckTiming) Error() string {
	return fmt.Sprintf("ErrCheckTiming: %s: %s", e.Service, e.Why)
}
func (e ErrWaitTooShort) Error() string {
	return fmt.Sprintf("ErrWaitTooShort: %s: wait.timeout %dns < required %dns", e.Service, e.Have, e.Need)
}
func (e ErrSecretInEnvironment) Error() string {
	return fmt.Sprintf("ErrSecretInEnvironment: %s: %s", e.Service, e.Key)
}
func (e ErrRunAsInUserMode) Error() string {
	return fmt.Sprintf("ErrRunAsInUserMode: %s runs as %q, owner is %q", e.Service, e.RunAs, e.Owner)
}
func (e ErrForkingDaemon) Error() string {
	return fmt.Sprintf("ErrForkingDaemon: %s: %s", e.Layer, e.Service)
}
func (e ErrUntypedParam) Error() string {
	return fmt.Sprintf("ErrUntypedParam: %s: %s", e.Service, e.Param)
}
func (e ErrParamInShell) Error() string {
	return fmt.Sprintf("ErrParamInShell: %s: %s", e.Service, e.Param)
}
func (e ErrSchemaTooNew) Error() string {
	return fmt.Sprintf("ErrSchemaTooNew: baseline %d, running %d", e.Baseline, e.Running)
}
func (e ErrDelegationUid) Error() string {
	return fmt.Sprintf("ErrDelegationUid: %s (%s) for principal %s", e.Service, e.Direction, e.Principal)
}
func (e ErrGrantNotClosed) Error() string {
	return fmt.Sprintf("ErrGrantNotClosed: %s is granted to %s but %s reaches it by %s",
		e.Granted, e.Principal, e.Requirer, e.Edge)
}

// CompileError holds every refusal of one Compile, sorted by message, so the
// result does not depend on the order of the layers.
type CompileError struct{ Errs []error }

func (c *CompileError) Error() string {
	parts := make([]string, len(c.Errs))
	for i, e := range c.Errs {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

// Unwrap lets errors.As and errors.Is see every refusal.
func (c *CompileError) Unwrap() []error { return c.Errs }

// collector gathers refusals and returns them sorted and deduplicated.
type collector struct{ errs []error }

func (c *collector) add(e error) { c.errs = append(c.errs, e) }

func (c *collector) result() error {
	if len(c.errs) == 0 {
		return nil
	}
	type kv struct {
		s string
		e error
	}
	all := make([]kv, len(c.errs))
	for i, e := range c.errs {
		all[i] = kv{e.Error(), e}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].s < all[j].s })
	out := &CompileError{}
	for i, x := range all {
		if i > 0 && all[i-1].s == x.s {
			continue
		}
		out.Errs = append(out.Errs, x.e)
	}
	return out
}
