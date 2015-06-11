// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"os"
	"strings"
)

// IsContinuousIntegration returns true if it thinks it's running on a known CI
// service.
func IsContinuousIntegration() bool {
	// Refs:
	// - http://docs.travis-ci.com/user/environment-variables/
	// - http://docs.drone.io/env.html
	// - https://circleci.com/docs/environment-variables
	return os.Getenv("CI") == "true"
}

// Globals

// reverse reverses a string.
func reverse(s string) string {
	n := len(s)
	runes := make([]rune, n)
	for _, rune := range s {
		n--
		runes[n] = rune
	}
	return string(runes[n:])
}

func rsplitn(s, sep string, n int) []string {
	items := strings.SplitN(reverse(s), sep, n)
	l := len(items)
	for i := 0; i < l/2; i++ {
		j := l - i - 1
		items[i], items[j] = reverse(items[j]), reverse(items[i])
	}
	if l&1 != 0 {
		i := l / 2
		items[i] = reverse(items[i])
	}
	return items
}
