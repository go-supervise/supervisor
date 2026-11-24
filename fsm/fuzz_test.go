package fsm

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const fuzzDir = "testdata/fuzz/FuzzStep"

// FuzzStep drives the transition function with arbitrary token sequences on
// the mixed plan. Whatever the bytes, a step never panics, never mutates its
// input, never lands on an unreachable cell, and every world it produces
// satisfies the invariants. `go test` replays the committed corpus.
func FuzzStep(f *testing.F) {
	p := mixedPlan(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		toks := decodeBytes(data)
		if len(toks) > 64 {
			toks = toks[:64]
		}
		w := p.NewWorld()
		for _, tok := range toks {
			in := decode(p, w, tok)
			before := w.Clone()
			nw, r, err := p.Step(w, in)
			if !sameWorld(before, w) {
				t.Fatalf("Step changed its input world on %+v", tok)
			}
			switch err {
			case nil:
			case ErrUnreachable, ErrNoFixpoint, ErrTime:
				t.Fatalf("%v on %+v from %s", err, tok, key(w))
			default:
				continue
			}
			if msg := perServiceViolation(r); msg != "" {
				t.Fatal(msg)
			}
			checkInvariants(t, p, nw)
			w = nw
		}
	})
}

// corpusFiles is the corpus the search generates: for every cell it fires,
// the shortest token sequence that fires it first, as a Go fuzz corpus file.
func corpusFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, tr := range bigRun(t).hit {
		b := encode(tr)
		name := fmt.Sprintf("cell-%x", sha256.Sum256(b))[:21]
		files[name] = fmt.Sprintf("go test fuzz v1\n[]byte(%q)\n", string(b))
	}
	return files
}

// TestFuzzCorpusMatchesTheSearch: the committed corpus is exactly what the
// breadth-first search generates, so it cannot rot when the tables change.
func TestFuzzCorpusMatchesTheSearch(t *testing.T) {
	want := corpusFiles(t)
	if *update {
		if err := os.RemoveAll(fuzzDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(fuzzDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range want {
			if err := os.WriteFile(filepath.Join(fuzzDir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	entries, err := os.ReadDir(fuzzDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		t.Fatalf("%d corpus files committed, the search generates %d (run with -update after reading the diff)", len(entries), len(want))
	}
	for name, body := range want {
		got, err := os.ReadFile(filepath.Join(fuzzDir, name))
		if err != nil || string(got) != body {
			t.Fatalf("corpus file %s is missing or differs (%v)", name, err)
		}
	}
}
