package surface

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// notDirfd are system calls whose name ends in "at" (the mark of the *at family)
// but that take no directory descriptor, each with the reason.
var notDirfd = map[string]string{
	"creat": "takes a path only",
	"stat":  "takes a path only",
	"lstat": "takes a path only",
	"fstat": "takes an ordinary descriptor, not a directory one",

	"cachestat": "takes an ordinary descriptor",
	"shmat":     "a shared-memory attach",
	"ustat":     "takes a device number",
}

// dirfdByName are system calls that take a directory (or mount) descriptor
// without being named *at, and that no wrapper of x/sys/unix names a dirfd.
var dirfdByName = []string{
	"fanotify_mark", "open_by_handle_at", "name_to_handle_at", "open_tree",
	"open_tree_attr", "move_mount", "mount_setattr", "statx", "fspick",
	"file_getattr", "file_setattr",
	"setxattrat", "getxattrat", "listxattrat", "removexattrat",
}

func xsysDir(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "golang.org/x/sys").Output()
	if err != nil {
		t.Fatalf("go list x/sys: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// wrapperDirfd are the system calls that a wrapper of x/sys/unix gives a
// parameter named ...dirfd, read from the wrapper's source.
func wrapperDirfd(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(xsysDir(t), "unix", "zsyscall_linux*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no zsyscall_linux files (%v)", err)
	}
	out := map[string]bool{}
	for _, f := range files {
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			has := false
			for _, p := range fd.Type.Params.List {
				for _, n := range p.Names {
					has = has || strings.HasSuffix(strings.ToLower(n.Name), "dirfd")
				}
			}
			if !has {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && strings.HasPrefix(id.Name, "SYS_") {
					out[strings.ToLower(strings.TrimPrefix(id.Name, "SYS_"))] = true
				}
				return true
			})
		}
	}
	return out
}

// Every system call that takes a directory descriptor, on either architecture,
// is in rawDirfd: those named *at, those a wrapper of x/sys/unix gives a dirfd
// parameter, and the few in dirfdByName. A kernel call added by an x/sys/unix
// update without its row fails here instead of drifting.
func TestRawDirfdCoversEveryDirfdSyscall(t *testing.T) {
	need := map[string]string{}
	archs := []string{"amd64", "arm64"}
	for w := range wrapperDirfd(t) {
		// the wrappers of 32-bit systems name calls these two do not have
		for _, arch := range archs {
			if _, ok := SysNum(arch)[w]; ok {
				need[w] = "a wrapper of x/sys/unix has a dirfd parameter"
			}
		}
	}
	for _, arch := range archs {
		for name := range SysNum(arch) {
			if _, no := notDirfd[name]; !no && (strings.HasSuffix(name, "at") || strings.HasSuffix(name, "at2")) {
				need[name] = "named *at"
			}
		}
	}
	for _, name := range dirfdByName {
		need[name] = "listed in dirfdByName"
	}
	var missing []string
	for name, why := range need {
		if _, ok := rawDirfd[name]; !ok {
			missing = append(missing, name+" ("+why+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("rawDirfd lacks:\n%s", strings.Join(missing, "\n"))
	}
	// a row for a name that is no system call of either architecture guards nothing
	for name := range rawDirfd {
		found := false
		for _, arch := range archs {
			_, ok := SysNum(arch)[name]
			found = found || ok
		}
		if !found {
			t.Errorf("rawDirfd names %q, which is no system call of amd64 or arm64", name)
		}
	}
}
