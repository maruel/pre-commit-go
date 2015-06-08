// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
)

func mainImpl() error {
	flag.Parse()
	// TODO(maruel): Do something.
	return nil
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "sample-pre-commit-go-custom-check: %s\n", err)
		os.Exit(1)
	}
}
