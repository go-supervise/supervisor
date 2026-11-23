package supervisor

import (
	"regexp"
	"slices"
	"strings"
)

// CurrentSchema is the schema version this binary enforces.
const CurrentSchema = 1

// Class is a field class of SUPERVISOR.md section 1.7.
type Class uint8

const (
	ClassD      Class = iota + 1 // definition: recipe writes, site may only restate
	ClassP                       // parameter values and credential sources: site only
	ClassT                       // timing and counts (K2)
	ClassH                       // hardening
	ClassE                       // edges
	ClassG                       // instance: site only
	ClassPolicy                  // restart.on
)

// Unit of a K2 field.
type Unit uint8

const (
	UnitDuration Unit = iota + 1 // nanoseconds
	UnitCount
)

// Req says when a field is in the required set Q of section 1.5.
type Req uint8

const (
	ReqNone           Req = iota
	ReqAlways             // every service
	ReqIfRequireEdges     // wait.timeout: only with a require edge
	ReqCheck              // check.*: all or none
	ReqHardening          // satisfied by the baseline's totality
)

type fieldDef struct {
	Kind  Kind
	Class Class
	Unit  Unit
	Since int
	// Legacy is the legacy value of a field newer than schema 1 (N9): the
	// value whose enforcement equals the enforcement before the field existed.
	Legacy Value
	Enum   []string
	Needs  bool // a K6 needs set, excluded from all {}
	Req    Req
}

type protDef struct {
	Name  string
	Since int
}

// schema is the field table. Tests extend a copy to model a later version.
type schema struct {
	fields      map[string]fieldDef
	protections []protDef
}

const durationMax = int64(365 * 24 * 3600 * 1e9) // one year, so that sums cannot overflow
const countMax = int64(1<<31 - 1)

var osNames = []string{"darwin", "freebsd", "linux", "windows"}

func defaultSchema() *schema {
	d := func(k Kind, enum ...string) fieldDef {
		return fieldDef{Kind: k, Class: ClassD, Since: 1, Enum: enum}
	}
	req := func(f fieldDef) fieldDef { f.Req = ReqAlways; return f }
	dur := func(r Req) fieldDef {
		return fieldDef{Kind: K2, Class: ClassT, Unit: UnitDuration, Since: 1, Req: r}
	}
	cnt := fieldDef{Kind: K2, Class: ClassT, Unit: UnitCount, Since: 1, Req: ReqCheck}
	h := func(k Kind, needs bool) fieldDef {
		return fieldDef{Kind: k, Class: ClassH, Since: 1, Needs: needs, Req: ReqHardening}
	}
	opt := func(f fieldDef) fieldDef { f.Req = ReqNone; return f }
	s := &schema{fields: map[string]fieldDef{
		"kind":              req(d(K1, "simple", "oneshot")),
		"exec":              req(d(K1)),
		"working_dir":       d(K1),
		"run_as.user":       d(K1),
		"run_as.group":      d(K1),
		"readiness.channel": req(d(K1, "fd", "sd_notify", "probe", "none")),
		"stop.signal":       req(d(K1, "TERM", "INT", "HUP", "QUIT", "USR1", "USR2")),
		"restart.on":        {Kind: K1, Class: ClassPolicy, Since: 1, Enum: []string{"never", "on-failure", "always"}, Req: ReqAlways},
		"enabled":           {Kind: K1, Class: ClassG, Since: 1, Enum: []string{"true", "false"}, Req: ReqAlways},
		"terminal":          {Kind: K1, Class: ClassG, Since: 1, Enum: []string{"true", "false"}, Req: ReqAlways},

		"stop.timeout":  dur(ReqAlways),
		"kill.timeout":  dur(ReqAlways),
		"start.timeout": dur(ReqAlways),
		"backoff.max":   dur(ReqAlways),
		"wait.timeout":  dur(ReqIfRequireEdges),
		"check.timeout": dur(ReqCheck),
		"check.period":  dur(ReqCheck),
		"check.rise":    cnt,
		"check.fall":    cnt,

		"limits.nofile": h(K3, false),
		"limits.nproc":  h(K3, false),
		"limits.memory": h(K3, false),
		"limits.core":   h(K3, false),

		"allow.capabilities":     h(K5, false),
		"allow.syscalls":         h(K5, false),
		"allow.address_families": h(K5, false),
		"allow.read_write":       h(K5P, false),

		"needs.capabilities":     opt(h(K6, true)),
		"needs.syscalls":         opt(h(K6, true)),
		"needs.address_families": opt(h(K6, true)),
		"read_only":              opt(h(K6, false)),
	}}
	for _, n := range []string{"nnp", "landlock", "seccomp"} {
		s.protections = append(s.protections, protDef{Name: n, Since: 1})
	}
	return s
}

func (s *schema) clone() *schema {
	c := &schema{fields: map[string]fieldDef{}, protections: slices.Clone(s.protections)}
	for k, v := range s.fields {
		c.fields[k] = v
	}
	return c
}

func (s *schema) protection(name string) (protDef, bool) {
	for _, p := range s.protections {
		if p.Name == name {
			return p, true
		}
	}
	return protDef{}, false
}

// blocks reports whether name is a plain block of the schema (one that
// holds fields named name.attr).
func (s *schema) isBlock(name string) bool {
	for k := range s.fields {
		if strings.HasPrefix(k, name+".") {
			return true
		}
	}
	return false
}

// pathOf names one field of a service: dynamic fields carry a slash.
func classOf(s *schema, path string) (fieldDef, bool) {
	if f, ok := s.fields[path]; ok {
		return f, true
	}
	switch {
	case strings.HasPrefix(path, "env/"):
		return fieldDef{Kind: K1, Class: ClassD, Since: 1}, true
	case strings.HasPrefix(path, "paramdecl/"):
		return fieldDef{Kind: K1, Class: ClassD, Since: 1}, true
	case strings.HasPrefix(path, "param/"):
		return fieldDef{Kind: K1, Class: ClassP, Since: 1}, true
	case strings.HasPrefix(path, "cred/") && strings.HasSuffix(path, "/source"):
		return fieldDef{Kind: K1, Class: ClassP, Since: 1}, true
	case strings.HasPrefix(path, "cred/"):
		return fieldDef{Kind: K1, Class: ClassD, Since: 1}, true
	case strings.HasPrefix(path, "edge/"):
		return fieldDef{Kind: K7, Class: ClassE, Since: 1}, true
	case strings.HasPrefix(path, "protect/") && strings.HasSuffix(path, "/exempt_on"):
		name := strings.TrimSuffix(strings.TrimPrefix(path, "protect/"), "/exempt_on")
		p, ok := s.protection(name)
		return fieldDef{Kind: K5, Class: ClassH, Since: p.Since, Req: ReqHardening}, ok
	case strings.HasPrefix(path, "protect/"):
		name := strings.TrimPrefix(path, "protect/")
		p, ok := s.protection(name)
		return fieldDef{Kind: K4, Class: ClassH, Since: p.Since, Req: ReqHardening,
			Legacy: Level(K4, LevelWaived)}, ok
	}
	return fieldDef{}, false
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,128}$`)
var envKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// pathAlphabet is the restricted alphabet of path parameter values (R3-L4).
var pathAlphabet = regexp.MustCompile(`^[A-Za-z0-9._/+@,=][A-Za-z0-9._/+@,=-]*$`)

var secretWords = []string{"PASSWORD", "PASSWD", "SECRET", "TOKEN", "CREDENTIAL", "PRIVATE_KEY", "API_KEY", "APIKEY"}

func looksSecret(key string) bool {
	u := strings.ToUpper(key)
	for _, w := range secretWords {
		if strings.Contains(u, w) {
			return true
		}
	}
	return false
}

var shells = []string{"sh", "bash", "dash", "zsh", "cmd.exe", "cmd", "powershell", "powershell.exe", "pwsh", "pwsh.exe"}
