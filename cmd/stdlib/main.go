// Package main contains the stdlib command.
package main

import (
	"github.com/abemedia/stdlib"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(stdlib.NewAnalyzer())
}
