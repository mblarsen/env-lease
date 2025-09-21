package provider

import "os/exec"

// execer is an interface to allow mocking of exec.Command.
type execer interface {
	Command(name string, arg ...string) *exec.Cmd
}

type realExecer struct{}

func (e *realExecer) Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

// Overridable for testing.
var cmdExecer execer = &realExecer{}
