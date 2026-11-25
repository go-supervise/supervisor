package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func setArgs(t *testing.T, a ...string) {
	t.Helper()
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = a
}

func swap(t *testing.T, code int) *[]int {
	t.Helper()
	oldExit, oldRun := osExit, runHelper
	t.Cleanup(func() { osExit, runHelper = oldExit, oldRun })
	exits := &[]int{}
	osExit = func(c int) { *exits = append(*exits, c) }
	runHelper = func(io.Writer) int { return code }
	return exits
}

func TestHelperExitCodeIsTheProcessExitCode(t *testing.T) {
	exits := swap(t, 7)
	root := RootCmd()
	root.SetArgs([]string{"helper"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(*exits) != 1 || (*exits)[0] != 7 {
		t.Errorf("exits %v, want [7]", *exits)
	}
}

func TestHelperSuccessDoesNotExit(t *testing.T) {
	exits := swap(t, 0)
	root := RootCmd()
	root.SetArgs([]string{"helper"})
	if err := root.Execute(); err != nil || len(*exits) != 0 {
		t.Errorf("err %v exits %v", err, *exits)
	}
}

func TestHelperTakesNoArguments(t *testing.T) {
	exits := swap(t, 0)
	root := RootCmd()
	var out bytes.Buffer
	root.SetErr(&out)
	root.SetArgs([]string{"helper", "extra"})
	if err := root.Execute(); err == nil || len(*exits) != 0 {
		t.Errorf("err %v exits %v", err, *exits)
	}
}

func TestMainExitsOneOnACommandError(t *testing.T) {
	exits := swap(t, 0)
	setArgs(t, "svd", "nonesuch")
	main()
	if len(*exits) != 1 || (*exits)[0] != 1 {
		t.Errorf("exits %v, want [1]", *exits)
	}
}

func TestMainSucceedsQuietly(t *testing.T) {
	exits := swap(t, 0)
	setArgs(t, "svd")
	main()
	if len(*exits) != 0 {
		t.Errorf("exits %v", *exits)
	}
}
