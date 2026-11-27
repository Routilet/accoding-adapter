package main

import (
	"fmt"
	"github.com/Routilet/accoding-adapter/internal/wire"
	"os"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--stdio" {
		fmt.Fprintln(os.Stderr, "usage: accoding-adapter --stdio")
		os.Exit(2)
	}
	if err := wire.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "adapter protocol stream closed")
		os.Exit(1)
	}
}
