package main

import (
	"os"

	routercmd "github.com/meistro2k/cosmo/router/cmd"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "query-plan" {
		routercmd.PlanGenerator(os.Args[1:])
	} else {
		routercmd.Main()
	}
}
