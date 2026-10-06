package main

import (
	"os/exec"
)

func platformOpenBrowser(url string) error { return exec.Command("xdg-open", url).Run() }
