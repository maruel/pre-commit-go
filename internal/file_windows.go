// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Imported from:
// Server: https://go.googlesource.com/go
// Revision: 883bc6ed0ea815293fe6309d66f967ea60630e87
// License: BSD
//
// Then modified to enable readonly file deletion on Windows.

package internal

import (
	"os"
	"syscall"
)

func Remove(name string) error {
	p, e := syscall.UTF16PtrFromString(name)
	if e != nil {
		return &os.PathError{"remove", name, e}
	}

	// Go file interface forces us to know whether
	// name is a file or directory. Try both.
	e = syscall.DeleteFile(p)
	if e == nil {
		return nil
	}
	e1 := syscall.RemoveDirectory(p)
	if e1 == nil {
		return nil
	}

	fi, err := os.Lstat(name)
	if err != nil {
		return err
	}

	if !fi.IsDir() {
		if fi.Mode().Perm() != 0777 {
			_ = os.Chmod(name, 0777)
			if err = syscall.DeleteFile(p); err == nil {
				return err
			}
		}
	}
	// TODO(maruel): Add DACL handling.
	return &os.PathError{"remove", name, e}
}
