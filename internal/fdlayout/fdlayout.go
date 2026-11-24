// Package fdlayout fixes the descriptor numbers of the spawn path
// (SUPERVISOR.md 7.1.1): M4's Files set H, and the stage's set S.
package fdlayout

// The helper's set H, installed by Go's fork child (C1) with dup3.
const (
	DevNull  = 0 // /dev/null
	LogW     = 1 // log pipe write end (also fd 2)
	LogW2    = 2
	Plan     = 3 // sealed plan memfd (H2)
	StatusW  = 4 // status pipe write end
	ReadyFD  = 5 // readiness (notify socket)
	StageFD  = 6 // O_PATH fd of the drop stage
	HelperFD = 7 // k: O_PATH fd of the helper itself (T17); last entry of Files
	NFiles   = 8 // len(Files)
)

// The stage's set S, laid out by H10.
const (
	StageStatus    = 3
	StageReady     = 4
	StageFrame     = 5
	StageCwd       = 6
	StageFirstFree = 7 // everything from here on is CLOEXEC at the stage's exec
)

// Status records on the status pipe.
const (
	StatusPerm  = 'P'
	StatusArmed = 'A'
)

// Fixed exit codes of the helper steps: all outside {1, 2} (7.1.1 H10).
const (
	ExitH1  = 11
	ExitH2  = 12
	ExitH3  = 13
	ExitH4  = 14
	ExitH5  = 15
	ExitH6  = 16
	ExitH7  = 17
	ExitH8  = 18
	ExitH9  = 19
	ExitH10 = 20
)
