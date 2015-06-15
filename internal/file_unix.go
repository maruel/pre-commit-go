// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build !windows

package internal

import "os"

// Remove calls back os.Remove() on non-Windows.
func Remove(name string) error {
	return os.Remove(name)
}
