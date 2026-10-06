package main

import (
	"os/exec"
)

func platformOpenBrowser(url string) error { return exec.Command("/usr/bin/open", url).Run() }
