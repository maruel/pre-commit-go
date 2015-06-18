// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/maruel/pre-commit-go/internal/pcg"
)

func main() {
	if err := pcg.MainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "pcg: %s\n", err)
		os.Exit(1)
	}
}
