// Copyright 2016 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build go1.4

package checks

import "os"

func init() {
	for _, i := range []string{"GIT_WORK_TREE", "GIT_DIR", "GIT_PREFIX"} {
		_ = os.Unsetenv(i)
	}
}
