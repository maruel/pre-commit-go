// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"testing"
	"time"

	"github.com/maruel/ut"
)

func TestRound(t *testing.T) {
	t.Parallel()
	ut.AssertEqual(t, 1500*time.Millisecond, round(1549*time.Millisecond, 100*time.Millisecond))
	ut.AssertEqual(t, 1600*time.Millisecond, round(1550*time.Millisecond, 100*time.Millisecond))
	ut.AssertEqual(t, -1500*time.Millisecond, round(-1549*time.Millisecond, 100*time.Millisecond))
	ut.AssertEqual(t, -1600*time.Millisecond, round(-1550*time.Millisecond, 100*time.Millisecond))
}
