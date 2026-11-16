//go:build !windows

package engine

import "os/exec"

func hideWindow(*exec.Cmd) {}

// makeSparse is a no-op: APFS and ext4 create sparse files on their own.
func makeSparse(any) {}
