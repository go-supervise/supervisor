//go:build linux

// Package keyring is step D5b of SUPERVISOR.md 7.1.1 (SECURITY-1 T12): the
// thread joins a new, empty, anonymous session keyring, in every mode.
package keyring

import "golang.org/x/sys/unix"

// JoinFresh is keyctl(KEYCTL_JOIN_SESSION_KEYRING, NULL). The caller must
// be on one OS thread (the stage has one), and must be root so the keyring
// is charged to root's quota.
func JoinFresh() (id int, err error) {
	// arg2 = 0 is the NULL name: an anonymous keyring (an empty string
	// would name one).
	return unix.KeyctlInt(unix.KEYCTL_JOIN_SESSION_KEYRING, 0, 0, 0, 0)
}

// Search is KEYCTL_SEARCH of a user key by description from the session
// keyring; it returns the key id or the error (ENOKEY when absent).
func Search(desc string) (int, error) {
	return unix.KeyctlSearch(unix.KEY_SPEC_SESSION_KEYRING, "user", desc, 0)
}
