//go:build !linux

package main

import "os/exec"

func configureReplayProcessGroup(_ *exec.Cmd) {}
