package main

import (
	"github.com/UnitVectorY-Labs/ghaqueue/internal/app"
	"os"
)

var Version = "dev"

func main() { os.Exit(app.Run(os.Args[1:], Version, os.Stdout, os.Stderr)) }
