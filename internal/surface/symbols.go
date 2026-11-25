package surface

import (
	"debug/dwarf"
	"debug/elf"
	"errors"
	"fmt"
)

// SymbolAbsence is half 3 of 7.1.2: neither the ELF symbol table nor the debug
// information of the shipped svd-monitor may name any of Absent. The debug
// information is read too because a function inlined at every call site has no
// symbol, only an abstract subprogram entry. It is exact about the functions it
// lists and blind to a raw call inside a function it does not list (half 1's
// job). A stripped binary, or one without debug information, is an error, not a
// pass: an empty table proves nothing.
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
	d, err := f.DWARF()
	if err != nil {
		return nil, fmt.Errorf("surface: %s has no debug information (-w?): %w", path, err)
	}
	if err := dwarfFunctions(d, have); err != nil {
		return nil, fmt.Errorf("surface: %s: reading debug information: %w", path, err)
	}
	return absence(have)
}

// dwarfFunctions adds every subprogram name of d to have, abstract
// (fully inlined) ones included.
func dwarfFunctions(d *dwarf.Data, have map[string]bool) error {
	r := d.Reader()
	for {
		e, err := r.Next() // (nil, nil) marks the end
		if err != nil {
			return err
		}
		if e == nil {
			return nil
		}
		if e.Tag == dwarf.TagSubprogram {
			if n, ok := e.Val(dwarf.AttrName).(string); ok {
				have[n] = true
			}
		}
	}
}

// absence judges a symbol set: it must be a Go executable's, and hold none of
// Absent.
func absence(have map[string]bool) (*Result, error) {
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
