// Package fsm is the lifecycle state machine of SUPERVISOR.md section 3
// (draft 13): a pure, deterministic function from a world and one input to a
// new world and a list of effects. It makes no OS call, starts no goroutine
// and reads no clock: time is a field of every input.
//
// The event table (11 states x 15 events), the guard grid (11 x 8, with
// Absent and Prio), the settle decision list and permits are DATA, in
// tables.go. The step function and the tests both read those values.
package fsm

import "fmt"

// State is the lifecycle state of one service (section 3.2).
type State uint8

// The 11 states, in the order of the event table's rows.
const (
	Inactive State = iota
	Waiting
	Starting
	Ready
	Done
	StopWait
	Stopping
	Killing
	Backoff
	Failed
	Leaked
	// NumStates is the number of states.
	NumStates = iota
)

// Same is the target of a cell that changes no state.
const Same State = 255

var stateNames = [...]string{"Inactive", "Waiting", "Starting", "Ready", "Done", "StopWait", "Stopping", "Killing", "Backoff", "Failed", "Leaked"}

func (s State) String() string {
	if s == Same {
		return "Same"
	}
	if int(s) >= len(stateNames) {
		return fmt.Sprintf("invalid(%d)", uint8(s))
	}
	return stateNames[s]
}

// Event is a column of the event table (section 3.4).
type Event uint8

// The 15 columns, in the table's order.
const (
	Ry      Event = iota // Retry
	Rs                   // Restart (operator)
	SEt                  // SpawnErr, transient
	SEp                  // SpawnErr, permanent
	X0                   // MainExit(ok)
	X1                   // MainExit(fail)
	TE                   // TreeEmpty
	Ri                   // Rise
	Fa                   // Fall
	WTO                  // WaitTO
	StartTO              // start.timeout
	SWTO                 // StopWaitTO
	StopTO               // stop.timeout
	KTO                  // kill.timeout
	BTO                  // BackoffTO
	// NumEvents is the number of event columns.
	NumEvents = iota
)

var eventNames = [...]string{"Ry", "Rs", "SEt", "SEp", "X0", "X1", "TE", "Ri", "Fa", "WTO", "StartTO", "SWTO", "StopTO", "KTO", "BTO"}

func (e Event) String() string { return eventNames[e] }

// Guard is a column of the guard grid (section 3.5).
type Guard uint8

// The 8 guards, in the grid's column order.
const (
	GUp Guard = iota
	GDown
	GOk
	GFailed
	GLost
	GFree
	GWexp
	GArm
	// NumGuards is the number of guard columns.
	NumGuards = iota
)

var guardNames = [...]string{"G_up", "G_down", "G_ok", "G_failed", "G_lost", "G_free", "G_wexp", "G_arm"}

func (g Guard) String() string { return guardNames[g] }

// Cause is why the current stop happens (section 3.2). CauseNone doubles as
// "assign nothing" in a Move.
type Cause uint8

// The causes.
const (
	CauseNone Cause = iota
	CauseDep
	CauseRestart
	CauseFail
	CauseNatural
)

var causeNames = [...]string{"none", "dep", "restart", "fail", "natural"}

func (c Cause) String() string { return causeNames[c] }

// Exit is the recorded status of the main process (D1, D7).
type Exit uint8

// The exit values. ExitSig is any exit recorded after term or kill was sent.
const (
	ExitNone Exit = iota
	ExitOK
	ExitFail
	ExitPerm
	ExitSig
)

var exitNames = [...]string{"none", "ok", "fail", "perm", "sig"}

func (x Exit) String() string { return exitNames[x] }

// Reason is meaningful in Failed only.
type Reason uint8

// The reasons.
const (
	ReasonNone Reason = iota
	ReasonExited
	ReasonSpawn
	ReasonIntensity
	ReasonWait
	ReasonDep
	ReasonUnkillable
)

var reasonNames = [...]string{"none", "exited", "spawn", "intensity", "wait", "dep", "unkillable"}

func (r Reason) String() string { return reasonNames[r] }

// Kind is the service kind.
type Kind uint8

// The kinds.
const (
	Daemon Kind = iota
	Oneshot
)

// RestartOn is restart.on (K1); Permits reads it.
type RestartOn uint8

// The restart policies.
const (
	Never RestartOn = iota
	OnFailure
	Always
)

// Mode is the instance mode. Quiescing carries World.Status.
type Mode uint8

// The modes.
const (
	Running Mode = iota
	Quiescing
)

// Timer names a state timer (and the liveness deadline), one bit each in
// SvcState.Live.
type Timer uint8

// The timers.
const (
	TWait Timer = iota
	TStart
	TSW
	TStop
	TKill
	TBackoff
	TLiveness
)

// Reach is the reachability class of a cell (section 3.8). The zero value is
// invalid, so a forgotten cell fails the totality test.
type Reach uint8

// The reach values.
const (
	ReachInvalid        Reach = iota
	Reachable                 // a live cell
	UnreachableFilter         // the epoch, generation or tree filter makes it impossible
	UnreachableO2             // exit before empty
	UnreachableLiveness       // liveness not armed
	Absent                    // guard grid only: not a guard of this row
)

var reachNames = [...]string{"INVALID", "live", "filter", "O2", "liveness", "absent"}

func (r Reach) String() string {
	if int(r) >= len(reachNames) {
		return fmt.Sprintf("invalid(%d)", uint8(r))
	}
	return reachNames[r]
}

// Cond is a conjunction of predicates, as a bitset.
type Cond uint16

// The predicates a cell or a settle rule may read.
const (
	CGoalUp Cond = 1 << iota
	CGoalDown
	CPending
	CReasonRecoverable
	COneshot
	CExitOK
	CExitFail
	CExitPerm
	CCauseDepRestartNone
	CCauseFail
	CCauseNatural
	CPermits
)

// ActionSet is a bitset of named actions (section 3.8's ActionSet).
type ActionSet uint32

// The actions.
const (
	ARecOK         ActionSet = 1 << iota // record(ok)
	ARecFail                             // record(fail), or perm from the status pipe
	ARecFailNoPipe                       // record(fail); the pipe cannot hold perm (Ready)
	ARecSig                              // record(sig)
	ARecSigPerm                          // record(sig), or perm from the status pipe
	ASettle                              // decide by settle(A), on TreeEmpty
	ASpawn                               // spawn
	ATerm                                // term
	AKill                                // kill
	ARetryPay                            // R_A := empty; pay(payer(A))
	AArmLiveness                         // arm the liveness deadline
	AArmWait                             // armed := true; arm WTO with the current gen
	ASetWexp                             // wexp := true
	AReport                              // report Move.Report
	ADelayMax                            // Backoff delay is backoff.max, uncharged
	ADelayRestart                        // Backoff delay is min(initial*factor^(|R_A|-1), max)
)

// Report kinds.
const (
	RepOverrun        = iota + 1 // StopWaitTO fired: T2 overrun
	RepLeaked                    // kill.timeout fired: the tree is leaked
	RepSpawnTransient            // transient spawn failure
)

// Move is what a cell does when it fires.
type Move struct {
	Target  State  // or Same
	Reason  Reason // for a Failed target
	Cause   Cause  // cause := Cause if cause = none; CauseNone assigns nothing
	Force   bool   // assign Cause even when cause is set
	Actions ActionSet
	Report  int
}

// Cell is one cell of the event table, of the guard grid, or a row of settle.
type Cell struct {
	Reach Reach
	Prio  uint8     // guard grid only: order within the row, unique
	Pre   ActionSet // run before the move (record, wexp)
	Cond  Cond      // zero: unconditional
	Then  Move
	Else  Move // taken when Cond is non-zero and does not hold
}

// SettleRule is one entry of settle's decision list (section 3.2).
type SettleRule struct {
	Name   string
	Cond   Cond
	Charge bool // charge(A) first, then overflow(A)
	Then   Move // the move when there is no overflow
	Over   Move // the move on Overflow; used only when Charge
}

// EffectKind is the kind of an Effect.
type EffectKind uint8

// The effects, for the executor.
const (
	EfSpawn   EffectKind = iota + 1
	EfTerm               // send stop.signal to the whole tree
	EfKill               // SIGKILL the tree
	EfArm                // arm a timer, tagged with the generation
	EfReport             // a report, Effect.Report
	EfQuiesce            // the instance entered Quiescing
)

// Effect is one action for the executor.
type Effect struct {
	Kind   EffectKind
	Svc    int
	Epoch  uint32
	Gen    uint32
	Timer  Timer
	Delay  int64
	Report int
	Status int
}

// MoveRec is one guard move of a propagation, for the bound of section 3.5.
type MoveRec struct {
	Svc      int
	From, To State
	Guard    Guard
	Sig      uint8 // the inputs of Svc's guards when it moved
}
