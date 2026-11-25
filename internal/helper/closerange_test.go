package helper

import (
	"os"
	"strings"
	"testing"
)

// close_range takes an unsigned int: the last descriptor is 0xFFFFFFFF on every
// word size. Deriving it by shifting ^uintptr(0) gives 0 on a 32-bit build,
// which closes nothing and fails the call, so no source may spell it that way.
func TestCloseRangeBoundIsNotDerivedFromTheWordSize(t *testing.T) {
	for _, f := range []string{"helper_linux.go", "kernel_linux.go", "h6_linux.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if s := strings.ReplaceAll(string(b), " ", ""); strings.Contains(s, "^uintptr(0)>>32") {
			t.Errorf("%s derives close_range's last descriptor from the word size", f)
		}
	}
	b, err := os.ReadFile("kernel_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "const closeRangeLast = uintptr(0xFFFFFFFF)") {
		t.Error("closeRangeLast is not the kernel's unsigned int maximum")
	}
}
