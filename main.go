package main

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
)

var AppVersion = "0.0.1"
var AppGitCommit = ""
var AppGitState = ""

func Version() string {
	version := AppVersion
	if len(AppGitCommit) > 0 {
		version += "-"
		version += AppGitCommit[0:8]
	}
	if len(AppGitState) > 0 && AppGitState != "clean" {
		version += "-"
		version += AppGitState
	}
	return version
}

func main() {
	o := nodeup.New(Version())
	o.Init()
}