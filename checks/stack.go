// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/maruel/panicparse/stack"
)

func CalcLengths(buckets stack.Buckets) (int, int) {
	srcLen := 0
	pkgLen := 0
	for _, bucket := range buckets {
		for _, line := range bucket.Signature.Stack {
			l := len(line.SourceLine())
			if l > srcLen {
				srcLen = l
			}
			l = len(line.Func.PkgName())
			if l > pkgLen {
				pkgLen = l
			}
		}
	}
	return srcLen, pkgLen
}

func PrettyStack(r *stack.Signature, srcLen, pkgLen int) string {
	out := []string{}
	for _, line := range r.Stack {
		s := fmt.Sprintf(
			"    %-*s %-*s %s(%s)",
			pkgLen, line.Func.PkgName(), srcLen, line.SourceLine(), line.Func.Name(),
			line.Args)
		out = append(out, s)
	}
	if r.StackElided {
		out = append(out, "    (...)")
	}
	return strings.Join(out, "\n")
}

func processStackTrace(data string) string {
	out := &bytes.Buffer{}
	goroutines, err := stack.ParseDump(bytes.NewBufferString(data), out)
	if err != nil || len(goroutines) == 0 {
		return data
	}
	buckets := stack.SortBuckets(stack.Bucketize(goroutines, true))
	srcLen, pkgLen := CalcLengths(buckets)
	for _, bucket := range buckets {
		extra := ""
		created := bucket.CreatedBy.Func.PkgDotName()
		if created != "" {
			if srcName := bucket.CreatedBy.SourceLine(); srcName != "" {
				created += " @ " + srcName
			}
			extra += " [Created by " + created + "]"
		}

		fmt.Fprintf(out, "%d: %s%s\n", len(bucket.Routines), bucket.State, extra)
		fmt.Fprintf(out, "%s\n", PrettyStack(&bucket.Signature, srcLen, pkgLen))
	}
	return out.String()
}
