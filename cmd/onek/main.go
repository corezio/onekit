package main

import (
	"fmt"
	"os"

	"github.com/1homsi/onekit/internal/onek"
)

const (
	minArgs  = 2
	dirArgAt = 3
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: onek <build|check|generate> [dir]")
}

func main() {
	if len(os.Args) < minArgs {
		usage()
		os.Exit(1)
	}

	dir := "."
	if len(os.Args) >= dirArgAt {
		dir = os.Args[2]
	}

	var err error
	switch os.Args[1] {
	case "build", "generate":
		err = onek.Build(dir)
	case "check":
		err = onek.Check(dir)
	case "fmt":
		fmt.Fprintln(os.Stderr, "onek fmt: not yet implemented")
		os.Exit(1)
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "onek:", err)
		os.Exit(1)
	}
}
