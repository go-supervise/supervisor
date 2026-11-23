package supervisor

import (
	"errors"
	"fmt"
	"testing"
)

// The fixture: one baseline, a recipe with one or more services, and a site
// layer that enables them. Every scenario edits one of the three.

const baseline = `
schema = 1
api {
  admin = ["alice"]
  read  = ["alice", "bob"]
}
all {
  limits {
    nofile = 1024
    nproc  = 100
    memory = 1000000
    core   = 0
  }
  allow {
    capabilities     = ["CAP_NET_BIND_SERVICE", "CAP_CHOWN"]
    syscalls         = ["read", "write", "futex"]
    address_families = ["inet", "unix"]
    read_write       = ["/var/lib"]
  }
  protect "nnp" {
    level     = "required"
    exempt_on = ["windows"]
  }
  protect "landlock" {
    level     = "required"
    exempt_on = ["darwin", "windows"]
  }
  protect "seccomp" {
    level     = "waived"
    exempt_on = ["darwin", "windows"]
  }
  stop {
    timeout = { min = "1s", max = "60s" }
  }
}
`

// svcRecipe is a complete recipe service. extra is spliced into the block.
func svcRecipe(name, extra string) string {
	return fmt.Sprintf(`
service %q {
  kind = "simple"
  exec = ["/bin/%s", "--port", "{{port}}"]
  readiness {
    channel = "fd"
  }
  stop {
    signal  = "TERM"
    timeout = { min = "5s", max = "120s", prefer = "30s" }
  }
  kill {
    timeout = "5s"
  }
  start {
    timeout = { min = "1s", max = "30s", prefer = "10s" }
  }
  backoff {
    max = "10s"
  }
  restart {
    on = "on-failure"
  }
  param "port" {
    type = "port"
  }
  %s
}
`, name, name, extra)
}

// svcSite enables a service and binds its port.
func svcSite(name, extra string) string {
	return fmt.Sprintf(`
service %q {
  enabled  = true
  terminal = false
  params   = { port = "8080" }
  %s
}
`, name, extra)
}

func rec(name, src string) Layer {
	return Layer{Name: name, Authority: Recipe, Admitted: true, Source: []byte(src)}
}

func site(name, src string) Layer {
	return Layer{Name: name, Authority: Site, Admitted: true, Source: []byte(src)}
}

// base returns a compiling set of layers for the service "web".
func base() []Layer {
	return []Layer{
		rec("recipe.hcl", svcRecipe("web", "")),
		site("site.hcl", baseline+svcSite("web", "")),
	}
}

func compile(layers ...Layer) (*Plan, error) { return Compile(layers, Options{}) }

func mustCompile(t *testing.T, layers ...Layer) *Plan {
	t.Helper()
	p, err := compile(layers...)
	if err != nil {
		t.Fatalf("Compile refused: %v", err)
	}
	return p
}

// wantErr asserts that Compile refuses with an error of type E, and returns it.
func wantErr[E error](t *testing.T, layers []Layer, opts ...Options) E {
	t.Helper()
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	p, err := Compile(layers, o)
	var e E
	if err == nil {
		t.Fatalf("Compile accepted, want %T (plan %v)", e, p != nil)
	}
	if !errors.As(err, &e) {
		t.Fatalf("Compile refused with %v, want %T", err, e)
	}
	return e
}

// with replaces the layer called name.
func with(layers []Layer, name, src string) []Layer {
	out := append([]Layer(nil), layers...)
	for i := range out {
		if out[i].Name == name {
			out[i].Source = []byte(src)
		}
	}
	return out
}

// plus appends a site layer.
func plus(layers []Layer, l Layer) []Layer { return append(append([]Layer(nil), layers...), l) }
