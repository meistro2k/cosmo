package main

import (
	routercmd "github.com/meistro2k/cosmo/router/cmd"
	// Import your modules here
	_ "github.com/meistro2k/cosmo/router/cmd/custom/module"
)

func main() {
	routercmd.Main()
}
