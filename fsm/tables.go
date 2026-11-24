package fsm

// Tables is the whole of section 3's data: the code (step.go) and the tests
// both read it. A Plan holds a copy, so a test can mutate one cell.
type Tables struct {
	Event  [NumStates][NumEvents]Cell // section 3.4, 165 cells
	Grid   [NumStates][NumGuards]Cell // section 3.5, 88 cells, 15 present
	Settle []SettleRule               // section 3.2, first match wins
}

// Default returns a fresh copy of the draft-13 tables.
func Default() Tables {
	t := Tables{Event: eventTable, Grid: buildGrid(gridCells), Settle: append([]SettleRule(nil), settleRules...)}
	return t
}

const (
	f  = UnreachableFilter
	o2 = UnreachableO2
	lv = UnreachableLiveness
)

func imp(r Reach) Cell { return Cell{Reach: r} }

func dot() Cell { return Cell{Reach: Reachable, Then: Move{Target: Same}} }

func act(m Move) Cell { return Cell{Reach: Reachable, Then: m} }

func rec(pre ActionSet, m Move) Cell { return Cell{Reach: Reachable, Pre: pre, Then: m} }

func stay() Move { return Move{Target: Same} }

func to(s State) Move { return Move{Target: s} }

// stopwait is stopwait(c): cause := c if cause = none, then StopWait.
func stopwait(c Cause) Move { return Move{Target: StopWait, Cause: c} }

func failed(r Reason) Move { return Move{Target: Failed, Reason: r} }

// eventTable is section 3.4, row by row, in the table's column order:
// Ry Rs SEt SEp X0 X1 TE Ri Fa WTO StartTO SWTO StopTO KTO BTO.
var eventTable = [NumStates][NumEvents]Cell{
	Inactive: {dot(), dot(), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f)},
	Waiting: {dot(), dot(), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f),
		// WTO: wexp := true; if pending -> Failed(wait), else no change (D3).
		{Reach: Reachable, Pre: ASetWexp, Cond: CPending, Then: failed(ReasonWait), Else: stay()},
		imp(f), imp(f), imp(f), imp(f), imp(f)},
	Starting: {dot(), act(stopwait(CauseRestart)),
		act(Move{Target: Backoff, Actions: ADelayMax | AReport, Report: RepSpawnTransient}),
		act(failed(ReasonSpawn)),
		rec(ARecOK, to(StopWait)), rec(ARecFail, to(StopWait)),
		imp(o2), act(Move{Target: Ready, Actions: AArmLiveness}), imp(lv),
		imp(f), act(stopwait(CauseFail)), imp(f), imp(f), imp(f), imp(f)},
	Ready: {dot(), act(stopwait(CauseRestart)), imp(f), imp(f),
		rec(ARecOK, to(StopWait)), rec(ARecFailNoPipe, to(StopWait)),
		imp(o2), dot(), act(stopwait(CauseFail)),
		imp(f), imp(f), imp(f), imp(f), imp(f), imp(f)},
	Done: {dot(), act(Move{Target: Waiting, Cause: CauseRestart, Force: true}),
		imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f)},
	StopWait: {dot(), dot(), imp(f), imp(f),
		rec(ARecOK, stay()), rec(ARecFail, stay()),
		act(Move{Target: Same, Actions: ASettle}), dot(), imp(f),
		imp(f), imp(f), act(Move{Target: Stopping, Actions: ATerm | AReport, Report: RepOverrun}), imp(f), imp(f), imp(f)},
	Stopping: {dot(), dot(), imp(f), imp(f),
		rec(ARecSig, stay()), rec(ARecSigPerm, stay()),
		act(Move{Target: Same, Actions: ASettle}), dot(), imp(f),
		imp(f), imp(f), imp(f), act(Move{Target: Killing, Actions: AKill}), imp(f), imp(f)},
	Killing: {dot(), dot(), imp(f), imp(f),
		rec(ARecSig, stay()), rec(ARecSigPerm, stay()),
		act(Move{Target: Same, Actions: ASettle}), dot(), imp(f),
		imp(f), imp(f), imp(f), imp(f), act(Move{Target: Leaked, Actions: AReport, Report: RepLeaked}), imp(f)},
	Backoff: {dot(), dot(), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f),
		imp(f), imp(f), imp(f), imp(f), imp(f), act(to(Waiting))},
	Failed: {
		// Ry and Rs: goal up -> R_A := empty; pay(payer(A)); Waiting, else no change.
		{Reach: Reachable, Cond: CGoalUp, Then: Move{Target: Waiting, Actions: ARetryPay}, Else: stay()},
		{Reach: Reachable, Cond: CGoalUp, Then: Move{Target: Waiting, Actions: ARetryPay}, Else: stay()},
		imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f), imp(f)},
	Leaked: {dot(), dot(), imp(f), imp(f),
		rec(ARecSig, stay()), rec(ARecSigPerm, stay()),
		act(failed(ReasonUnkillable)), dot(), imp(f),
		imp(f), imp(f), imp(f), imp(f), imp(f), imp(f)},
}

// gridCells are the 15 present cells of section 3.5; every other cell of the
// 88 is Absent. Prio is the order in the row, unique.
var gridCells = map[State]map[Guard]Cell{
	Inactive: {GUp: {Reach: Reachable, Prio: 1, Then: to(Waiting)}},
	Waiting: {
		GDown:   {Reach: Reachable, Prio: 1, Then: to(Inactive)},
		GFailed: {Reach: Reachable, Prio: 2, Then: failed(ReasonDep)},
		GWexp:   {Reach: Reachable, Prio: 3, Then: failed(ReasonWait)},
		GArm:    {Reach: Reachable, Prio: 4, Then: Move{Target: Same, Actions: AArmWait}},
		GOk:     {Reach: Reachable, Prio: 5, Then: Move{Target: Starting, Actions: ASpawn}},
	},
	Starting: {
		GDown: {Reach: Reachable, Prio: 1, Then: stopwait(CauseNone)},
		GLost: {Reach: Reachable, Prio: 2, Then: stopwait(CauseDep)},
	},
	Ready: {
		GDown: {Reach: Reachable, Prio: 1, Then: stopwait(CauseNone)},
		GLost: {Reach: Reachable, Prio: 2, Then: stopwait(CauseDep)},
	},
	Done:     {GDown: {Reach: Reachable, Prio: 1, Then: to(Inactive)}},
	StopWait: {GFree: {Reach: Reachable, Prio: 1, Then: Move{Target: Stopping, Actions: ATerm}}},
	Backoff:  {GDown: {Reach: Reachable, Prio: 1, Then: to(Inactive)}},
	Failed: {
		GDown: {Reach: Reachable, Prio: 1, Then: to(Inactive)},
		GOk:   {Reach: Reachable, Prio: 2, Cond: CReasonRecoverable | CGoalUp, Then: to(Waiting)},
	},
}

func buildGrid(cells map[State]map[Guard]Cell) [NumStates][NumGuards]Cell {
	var g [NumStates][NumGuards]Cell
	for s := range g {
		for k := range g[s] {
			if c, ok := cells[State(s)][Guard(k)]; ok {
				g[s][k] = c
			} else {
				g[s][k] = Cell{Reach: Absent}
			}
		}
	}
	return g
}

// settleRules is settle(A) of section 3.2, first match wins.
var settleRules = []SettleRule{
	{Name: "1 goal down", Cond: CGoalDown, Then: to(Inactive)},
	{Name: "2 perm", Cond: CExitPerm, Then: failed(ReasonSpawn)},
	{Name: "3 oneshot ok", Cond: COneshot | CExitOK, Then: to(Done)},
	{Name: "4 dep/restart/none, crash", Cond: CCauseDepRestartNone | CExitFail, Charge: true, Then: to(Waiting), Over: failed(ReasonIntensity)},
	{Name: "4 dep/restart/none", Cond: CCauseDepRestartNone, Then: to(Waiting)},
	{Name: "5 cause fail", Cond: CCauseFail, Charge: true, Then: Move{Target: Backoff, Actions: ADelayRestart}, Over: failed(ReasonIntensity)},
	{Name: "6 natural, permitted", Cond: CCauseNatural | CPermits, Charge: true, Then: Move{Target: Backoff, Actions: ADelayRestart}, Over: failed(ReasonIntensity)},
	{Name: "7 otherwise", Then: failed(ReasonExited)},
}

// Permits is restart.on read through permits(x): Permits[on][0] for ok,
// Permits[on][1] for fail (section 3.2).
var Permits = [3][2]bool{
	Never:     {false, false},
	OnFailure: {false, true},
	Always:    {true, true},
}

// StateTimer is the single timer a state arms on entry, when it has one.
// Waiting arms none: its WTO is armed by G_arm (E4). Ready arms the liveness
// deadline by the action AArmLiveness on the Rise that enters it.
var StateTimer = [NumStates]struct {
	Has   bool
	Timer Timer
}{
	Starting: {true, TStart},
	StopWait: {true, TSW},
	Stopping: {true, TStop},
	Killing:  {true, TKill},
	Backoff:  {true, TBackoff},
}

// EventTimer maps a timer event (and Fall, the liveness deadline) to the
// timer bit it consumes.
var EventTimer = [NumEvents]struct {
	Has   bool
	Timer Timer
}{
	Fa:      {true, TLiveness},
	WTO:     {true, TWait},
	StartTO: {true, TStart},
	SWTO:    {true, TSW},
	StopTO:  {true, TStop},
	KTO:     {true, TKill},
	BTO:     {true, TBackoff},
}

// Reads says which services a cell of a service C reads (section 3.8's static
// serialisation check).
type Reads uint8

// The read classes.
const (
	RSelf       Reads = 1 << iota
	RReq              // C's require-targets
	RTargets          // every target of C
	RDependents       // C's uncut dependents (G_free)
	RGoal             // goal(C): C's holders and the uncut dependents' closure
)

// GuardReads is what each guard reads.
var GuardReads = [NumGuards]Reads{
	GUp:     RSelf | RGoal,
	GDown:   RSelf | RGoal,
	GOk:     RSelf | RTargets,
	GFailed: RSelf | RReq,
	GLost:   RSelf | RReq,
	GFree:   RSelf | RDependents,
	GWexp:   RSelf | RReq,
	GArm:    RSelf | RReq,
}

// EventReads is what an event's cells read, over every state.
var EventReads = [NumEvents]Reads{
	Ry:      RSelf | RGoal,
	Rs:      RSelf,
	SEt:     RSelf,
	SEp:     RSelf,
	X0:      RSelf,
	X1:      RSelf,
	TE:      RSelf | RGoal,
	Ri:      RSelf,
	Fa:      RSelf,
	WTO:     RSelf | RReq,
	StartTO: RSelf,
	SWTO:    RSelf,
	StopTO:  RSelf,
	KTO:     RSelf,
	BTO:     RSelf,
}

// TimerDrain is the O5' drained set of a timer cell: the services whose
// pending work the executor drains before it delivers the timer.
var TimerDrain = [NumEvents]Reads{
	WTO:     RSelf | RReq,
	StartTO: RSelf,
	SWTO:    RSelf,
	StopTO:  RSelf,
	KTO:     RSelf,
	BTO:     RSelf,
}
