package main

import (
	"os"

	"github.com/dx111ge/homelabmon/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
