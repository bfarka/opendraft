package main

import (
	"fmt"
	"io"
	"os"
)

const version = "0.0.0-dev"

func run(w io.Writer) error {
	_, err := fmt.Fprintf(w, "opendraft %s\n", version)
	return err
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "opendraft: %v\n", err)
		os.Exit(1)
	}
}
