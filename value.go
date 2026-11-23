package supervisor

import (
	"slices"
	"sort"
)

// Kind is the field kind of SUPERVISOR.md section 1.3. Each kind is a
// meet-semilattice; K1 and K2 additionally carry the conflict element
// (the error element of section 1.2).
type Kind uint8

const (
	K1  Kind = iota + 1 // flat: equal values or conflict
	K2                  // interval, with prefer sets
	K3                  // bound: min
	K4                  // protection: chain required < waived < top
	K5                  // allow-set: intersection, unset is the universe
	K5P                 // path-region allow-set: symbolic, meet is the set of operands (M11)
	K6                  // demand-set: union
	K7                  // edge: chain require < want < order < none
)

// Chain levels of K4 and K7. The smaller level is the tighter one.
const (
	LevelRequired int64 = 0 // K4 required; K7 require
	LevelWaived   int64 = 1 // K4 waived;   K7 want
	LevelOrder    int64 = 2 // K7 order
)

// Value is one field's value in some layer or in a meet. Set == false is the
// top element: the layer says nothing about the field.
//
// The representation is canonical, so that Equal is structural equality and
// the laws of section 1.2 can be tested as equations on values.
type Value struct {
	Kind Kind
	Set  bool
	// K1: the distinct values stated, sorted. More than one is the conflict.
	// K5, K5P, K6: the members, sorted and distinct.
	Strs []string
	// K2: the closed interval [Lo, Hi]; the canonical empty interval is
	// {1, 0}. K3: the bound, in Lo. K4 and K7: the chain level, in Lo.
	Lo, Hi int64
	// K2 only: the prefer points, per authority (draft 7, D-M2). Each is a
	// K6 set, so it is a union.
	PrefR, PrefS []int64
}

// Top returns the top element of kind k.
func Top(k Kind) Value { return Value{Kind: k} }

// Flat returns a K1 value stating v.
func Flat(v string) Value { return Value{Kind: K1, Set: true, Strs: []string{v}} }

// Interval returns a K2 value for [lo, hi]. A reversed interval is the
// canonical empty one.
func Interval(lo, hi int64) Value {
	if lo > hi {
		lo, hi = 1, 0
	}
	return Value{Kind: K2, Set: true, Lo: lo, Hi: hi}
}

// Bound returns a K3 value.
func Bound(n int64) Value { return Value{Kind: K3, Set: true, Lo: n} }

// Level returns a K4 or K7 value.
func Level(k Kind, l int64) Value { return Value{Kind: k, Set: true, Lo: l} }

// SetOf returns a K5, K5P or K6 value with the given members.
func SetOf(k Kind, members ...string) Value {
	return Value{Kind: k, Set: true, Strs: sortedDistinct(members)}
}

func sortedDistinct(in []string) []string {
	if len(in) == 0 {
		return nil // a non-nil empty slice is not the canonical empty set
	}
	out := slices.Clone(in)
	sort.Strings(out)
	return slices.Compact(out)
}

func sortedDistinctInt(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

func union(a, b []string) []string { return sortedDistinct(append(slices.Clone(a), b...)) }

func intersect(a, b []string) []string {
	var out []string
	for _, x := range a {
		if _, ok := slices.BinarySearch(b, x); ok {
			out = append(out, x)
		}
	}
	return out
}

func subset(a, b []string) bool {
	for _, x := range a {
		if _, ok := slices.BinarySearch(b, x); !ok {
			return false
		}
	}
	return true
}

// Meet is the meet of two values of the same kind. It is associative,
// commutative and idempotent, has the top element as identity, and the
// conflict element absorbs (laws A, C, I, U, Z).
func Meet(a, b Value) Value {
	if a.Kind != b.Kind {
		panic("supervisor: meet of values of different kinds")
	}
	if !a.Set && !b.Set {
		return Value{Kind: a.Kind}
	}
	r := Value{Kind: a.Kind, Set: true}
	switch a.Kind {
	case K1, K5P, K6:
		r.Strs = union(a.Strs, b.Strs)
	case K5:
		switch {
		case !a.Set:
			r.Strs = slices.Clone(b.Strs)
		case !b.Set:
			r.Strs = slices.Clone(a.Strs)
		default:
			r.Strs = intersect(a.Strs, b.Strs)
		}
	case K2:
		switch {
		case !a.Set:
			r.Lo, r.Hi = b.Lo, b.Hi
		case !b.Set:
			r.Lo, r.Hi = a.Lo, a.Hi
		default:
			r.Lo, r.Hi = max(a.Lo, b.Lo), min(a.Hi, b.Hi)
		}
		if r.Lo > r.Hi {
			r.Lo, r.Hi = 1, 0
		}
		r.PrefR = sortedDistinctInt(append(slices.Clone(a.PrefR), b.PrefR...))
		r.PrefS = sortedDistinctInt(append(slices.Clone(a.PrefS), b.PrefS...))
	case K3, K4, K7:
		switch {
		case !a.Set:
			r.Lo = b.Lo
		case !b.Set:
			r.Lo = a.Lo
		default:
			r.Lo = min(a.Lo, b.Lo)
		}
	}
	if len(r.Strs) == 0 {
		r.Strs = nil
	}
	return r
}

// IsErr reports whether v is the conflict element: unequal K1 values, or an
// empty K2 interval.
func (v Value) IsErr() bool {
	switch v.Kind {
	case K1:
		return len(v.Strs) > 1
	case K2:
		return v.Set && v.Lo > v.Hi
	}
	return false
}

// Equal is structural equality.
func (v Value) Equal(w Value) bool {
	return v.Kind == w.Kind && v.Set == w.Set && v.Lo == w.Lo && v.Hi == w.Hi &&
		slices.Equal(v.Strs, w.Strs) && slices.Equal(v.PrefR, w.PrefR) && slices.Equal(v.PrefS, w.PrefS)
}

// Leq is the order of section 1.3, written independently of Meet: a is at
// least as restrictive as b. Prefer sets do not take part in the order.
func Leq(a, b Value) bool {
	if !b.Set {
		return true // everything is below the top
	}
	if !a.Set {
		return false
	}
	switch a.Kind {
	case K1:
		return subset(b.Strs, a.Strs)
	case K2:
		if a.Lo > a.Hi {
			return true // the conflict element is below every value
		}
		return b.Lo <= b.Hi && a.Lo >= b.Lo && a.Hi <= b.Hi
	case K3, K4, K7:
		return a.Lo <= b.Lo
	case K5:
		return subset(a.Strs, b.Strs)
	case K5P, K6:
		return subset(b.Strs, a.Strs)
	}
	return false
}
