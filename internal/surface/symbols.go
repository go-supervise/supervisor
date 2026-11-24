package surface

import (
	"debug/elf"
	"errors"
	"fmt"
)

// SymbolAbsence is half 3 of 7.1.2: the ELF symbol table of the shipped
// svd-monitor must hold none of Absent. It is exact about the functions it
// lists and blind to a raw call inside a function it does not list (half 1's
// job). A stripped binary is an error, not a pass: an empty table proves
// nothing.
func SymbolAbsence(path string) (*Result, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		return nil, fmt.Errorf("surface: %s has no symbol table (stripped?): %w", path, err)
	}
	have := make(map[string]bool, len(syms))
	for _, s := range syms {
		have[s.Name] = true
	}
	if !have["main.main"] {
		return nil, errors.New("surface: no main.main in the symbol table: not a Go executable, or stripped")
	}
	var res Result
	for _, n := range Absent {
		if have[n] {
			res.Failures = append(res.Failures, fmt.Sprintf("symbols: %s is linked into svd-monitor", n))
		}
	}
	return &res, nil
}
