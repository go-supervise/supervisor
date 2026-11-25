package surface

import (
	"errors"
	"go/version"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
)

// R_go is per toolchain: the committed tables name the one that generated
// them, and a build by another toolchain is refused.
func TestCheckToolchainRefusesAnotherToolchain(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		stamp := RGoToolchain(arch)
		if !strings.HasPrefix(stamp, "go1.") || strings.Count(stamp, ".") != 1 {
			t.Fatalf("rgo_%s.txt stamp %q is not a language version such as go1.27", arch, stamp)
		}
		if err := CheckToolchain(stamp+".4", arch); err != nil {
			t.Errorf("%s: its own toolchain refused: %v", arch, err)
		}
		if err := CheckToolchain("go1.25.9", arch); err == nil {
			t.Errorf("%s: go1.25.9 accepted", arch)
		}
		if err := CheckToolchain("go1.99rc1", arch); err == nil {
			t.Errorf("%s: go1.99rc1 accepted", arch)
		}
	}
	if err := CheckToolchain("go1.27.4", "riscv64"); err == nil || !strings.Contains(err.Error(), "no toolchain stamp") {
		t.Errorf("an architecture with no table: %v", err)
	}
}

func TestCheckBuildEnvRefusesExtraFlagsAndExperiments(t *testing.T) {
	env := os.Environ()
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOEXPERIMENT", "")
	if err := checkBuildEnv(".", env); err != nil {
		t.Fatalf("an empty environment: %v", err)
	}
	t.Setenv("GOFLAGS", "-mod=mod")
	if err := checkBuildEnv(".", os.Environ()); err == nil || !strings.Contains(err.Error(), "GOFLAGS") {
		t.Errorf("GOFLAGS set in the environment: %v", err)
	}
	t.Setenv("GOFLAGS", "-buildvcs=false")
	if err := checkBuildEnv(".", os.Environ()); err != nil {
		t.Errorf("-buildvcs=false changes no code: %v", err)
	}
	t.Setenv("GOFLAGS", "")
	old := goEnvOf
	defer func() { goEnvOf = old }()
	for _, k := range []string{"GOFLAGS", "GOEXPERIMENT"} {
		goEnvOf = func(_ string, _ []string, key string) (string, error) {
			if key == k {
				return "x", nil
			}
			return "", nil
		}
		if err := checkBuildEnv(".", nil); err == nil || !strings.Contains(err.Error(), k) {
			t.Errorf("%s set: %v", k, err)
		}
	}
	goEnvOf = func(string, []string, string) (string, error) { return "", errors.New("no go") }
	if err := checkBuildEnv(".", nil); err == nil {
		t.Error("an unreadable environment is not a pass")
	}
}

func TestCheckBuildInfoRefusesAnotherBuild(t *testing.T) {
	o := Options{GOARCH: "amd64", Tags: []string{"canary_new"}}
	good := func(edit map[string]string) *debug.BuildInfo {
		m := map[string]string{"CGO_ENABLED": "0", "GOARCH": "amd64", "GOOS": "linux", "-tags": "canary_new"}
		for k, v := range edit {
			m[k] = v
		}
		bi := &debug.BuildInfo{}
		for k, v := range m {
			bi.Settings = append(bi.Settings, debug.BuildSetting{Key: k, Value: v})
		}
		return bi
	}
	if err := CheckBuildInfo(good(nil), o); err != nil {
		t.Fatalf("the analysed build: %v", err)
	}
	if err := CheckBuildInfo(nil, o); err == nil {
		t.Error("no build information accepted")
	}
	for name, edit := range map[string]map[string]string{
		"cgo":        {"CGO_ENABLED": "1"},
		"arch":       {"GOARCH": "arm64"},
		"os":         {"GOOS": "darwin"},
		"tags":       {"-tags": "canary_never"},
		"experiment": {"GOEXPERIMENT": "simd"},
		"gcflags":    {"-gcflags": "all=-N"},
		"ldflags":    {"-ldflags": "-s"},
		"asmflags":   {"-asmflags": "-D x"},
		"race":       {"-race": "true"},
		"msan":       {"-msan": "true"},
		"asan":       {"-asan": "true"},
		"cover":      {"-cover": "true"},
	} {
		if err := CheckBuildInfo(good(edit), o); err == nil {
			t.Errorf("%s: a differently built binary accepted", name)
		}
	}
	if err := CheckBuildInfo(good(map[string]string{"-race": "false"}), o); err != nil {
		t.Errorf("-race=false is the default: %v", err)
	}
}

// The gate runs on the release toolchain: under another one the tables would be
// judged against a standard library they were not generated from.
func TestCommittedRGoNamesTheRunningToolchain(t *testing.T) {
	if testing.Short() {
		t.Skip("short lanes run on older toolchains too; the gate1 job runs it")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if got, want := RGoToolchain(arch), version.Lang(runtime.Version()); got != want {
			t.Errorf("rgo_%s.txt is stamped %q, this toolchain is %q", arch, got, want)
		}
	}
}

func TestListsAreDisjointWhereTheySay(t *testing.T) {
	never := Set(Never)
	for _, l := range [][]string{AMStart, AMSteady} {
		for _, n := range l {
			if never[n] {
				t.Errorf("%s is both allowed and never allowed", n)
			}
		}
	}
	steady := Set(AMSteady)
	for _, n := range AMStart {
		if steady[n] {
			t.Errorf("%s is in both A_M^start and A_M^steady", n)
		}
	}
}

func TestListsHaveNoDuplicates(t *testing.T) {
	for name, l := range map[string][]string{
		"AMStart": AMStart, "AMSteady": AMSteady, "Never": Never, "ForkChildGated": ForkChildGated,
		"AllowedAttr": AllowedAttr, "Absent": Absent,
	} {
		if len(Set(l)) != len(l) {
			t.Errorf("%s has a duplicate", name)
		}
	}
}

func TestForkChildGatedIsFencedByTheAttrCheck(t *testing.T) {
	// Every gated call is one a SysProcAttr field switches on; none of the
	// calls M_p itself issues in steady state is gated.
	gated := Set(ForkChildGated)
	for _, n := range []string{"recvmsg", "sendmsg", "waitid", "clone3", "execve", "setsid", "dup3", "openat"} {
		if gated[n] {
			t.Errorf("%s is M_p's own call, it cannot be gated", n)
		}
	}
	for _, f := range AllowedAttr {
		switch f {
		case "Setsid", "Cloneflags", "UseCgroupFD", "CgroupFD", "PidFD":
		default:
			t.Errorf("unexpected allowed field %s", f)
		}
	}
}

func TestEveryListedNameHasANumberOnBothArchitectures(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		tab := SysNum(arch)
		for name, l := range map[string][]string{"AMStart": AMStart, "AMSteady": AMSteady, "Never": Never} {
			for _, n := range l {
				if _, ok := tab[n]; !ok && !archSpecific[arch][n] {
					t.Errorf("%s %s has no number on %s", name, n, arch)
				}
			}
		}
	}
}

func TestCommittedSysNumTablesAreCurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("loads x/sys/unix (the gate1 job runs it)")
	}
	for arch, committed := range map[string]string{"amd64": sysnumAMD64, "arm64": sysnumARM64} {
		got, err := GenerateSysNum("../..", arch)
		if err != nil {
			t.Fatal(err)
		}
		if got != committed {
			t.Errorf("sysnum_%s.txt is out of date: run `syscallsurface sysnum --arch %s --out internal/surface/sysnum_%s.txt`", arch, arch, arch)
		}
	}
}

func TestSysNumParsesAndNamesReverse(t *testing.T) {
	if SysNum("riscv64") != nil {
		t.Error("an unknown architecture has no table")
	}
	if SysNum("amd64")["read"] != 0 || SysNum("amd64")["write"] != 1 || SysNum("arm64")["read"] != 63 {
		t.Errorf("amd64 %d %d arm64 %d", SysNum("amd64")["read"], SysNum("amd64")["write"], SysNum("arm64")["read"])
	}
	if sysNames("amd64")[1] != "write" || sysNames("arm64")[63] != "read" {
		t.Error("reverse table wrong")
	}
}

func TestParseListSkipsCommentsAndBlanksAndSorts(t *testing.T) {
	got := parseList("# comment\n\n  write  \nread\n#x\n")
	want := []string{"read", "write"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
}

func TestRGoLists(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		l := RGo(arch)
		if len(l) == 0 || !sort.StringsAreSorted(l) {
			t.Errorf("%s: %v", arch, l)
		}
	}
	if RGo("riscv64") != nil {
		t.Error("an unknown architecture has no list")
	}
}

func TestAllowedIsTheUnion(t *testing.T) {
	a := Allowed("amd64")
	for _, n := range append(append([]string{}, AMStart...), AMSteady...) {
		if !a[n] {
			t.Errorf("%s missing", n)
		}
	}
	for _, n := range RGo("amd64") {
		if !a[n] {
			t.Errorf("%s missing", n)
		}
	}
	if a["kill"] {
		t.Error("kill is allowed")
	}
}

func TestSet(t *testing.T) {
	s := Set([]string{"a", "b"})
	if len(s) != 2 || !s["a"] || !s["b"] || s["c"] {
		t.Fatalf("%v", s)
	}
}
