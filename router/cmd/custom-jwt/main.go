package main

import (
	routercmd "github.com/meistro2k/cosmo/router/cmd"
	_ "github.com/meistro2k/cosmo/router/cmd/custom-jwt/module"
)

func main() {
	routercmd.Main()
}
