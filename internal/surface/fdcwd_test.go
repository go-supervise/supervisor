package surface

import (
	"go/constant"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestIsFdcwd(t *testing.T) {
	cases := []struct {
		name string
		v    ssa.Value
		want bool
	}{
		{"signed", ssa.NewConst(constant.MakeInt64(-100), types.Typ[types.Int]), true},
		{"word", ssa.NewConst(constant.MakeUint64(^uint64(99)), types.Typ[types.Uintptr]), true},
		{"low32", ssa.NewConst(constant.MakeUint64(0xffffff9c), types.Typ[types.Uint32]), true},
		{"other", ssa.NewConst(constant.MakeInt64(5), types.Typ[types.Int]), false},
		{"negative other", ssa.NewConst(constant.MakeInt64(-99), types.Typ[types.Int]), false},
		{"string", ssa.NewConst(constant.MakeString("x"), types.Typ[types.String]), false},
		{"nil", ssa.NewConst(nil, types.Typ[types.UntypedNil]), false},
	}
	for _, c := range cases {
		if got := isFdcwd(c.v); got != c.want {
			t.Errorf("%s: isFdcwd = %v, want %v", c.name, got, c.want)
		}
	}
}
