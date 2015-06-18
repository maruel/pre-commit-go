// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Modified to add support to load files already in memory from a io.Reader
// instead of a filename to skip disk I/O altogether.
// Add support for methods.

// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cover

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
)

// FindFuncs returns all the functions defined by a Go source file.
func FindFuncs(fileName string, r io.Reader) ([]*FuncExtent, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, fileName, r, 0)
	if err != nil {
		return nil, err
	}
	visitor := &funcVisitor{fset: fset, fileName: fileName}
	ast.Walk(visitor, parsed)
	return visitor.funcs, nil
}

type FuncExtent struct {
	FileName  string
	FuncName  string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

type funcVisitor struct {
	fset     *token.FileSet
	fileName string
	funcs    []*FuncExtent
}

func (v *funcVisitor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.FuncDecl:
		start := v.fset.Position(n.Pos())
		end := v.fset.Position(n.End())
		name := n.Name.Name
		if n.Recv != nil {
			// A method.
			t := n.Recv.List[0].Type
			if s, ok := t.(*ast.StarExpr); ok {
				// Pointer receiver.
				t = s.X
			}
			if i, ok := t.(*ast.Ident); ok {
				name = i.Name + "." + name
			}
		}
		fe := &FuncExtent{
			FileName:  v.fileName,
			FuncName:  name,
			StartLine: start.Line,
			StartCol:  start.Column,
			EndLine:   end.Line,
			EndCol:    end.Column,
		}
		v.funcs = append(v.funcs, fe)
	}
	return v
}

func (f *FuncExtent) Coverage(profile *Profile) (num, den int64) {
	// We could avoid making this n^2 overall by doing a single scan and annotating the functions,
	// but the sizes of the data structures is never very large and the scan is almost instantaneous.
	var covered, total int64
	// The blocks are sorted, so we can stop counting as soon as we reach the end of the relevant block.
	for _, b := range profile.Blocks {
		if b.StartLine > f.EndLine || (b.StartLine == f.EndLine && b.StartCol >= f.EndCol) {
			// Past the end of the function.
			break
		}
		if b.EndLine < f.StartLine || (b.EndLine == f.StartLine && b.EndCol <= f.StartCol) {
			// Before the beginning of the function
			continue
		}
		total += int64(b.NumStmt)
		if b.Count > 0 {
			covered += int64(b.NumStmt)
		}
	}
	if total == 0 {
		total = 1 // Avoid zero denominator.
	}
	return covered, total
}
