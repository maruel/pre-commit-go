// Copyright 2015 Daniel Jacques. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
)

var errInvalidShard = errors.New("invalid shard specifier")

type shardFlag struct {
	current int
	total   int
}

var _ flag.Value = (*shardFlag)(nil)

func (s *shardFlag) currentShard() int {
	return s.current
}

func (s *shardFlag) totalShards() int {
	if s.total == 0 {
		return 1
	}
	return s.total
}

func (s *shardFlag) String() string {
	return fmt.Sprintf("%d/%d", s.currentShard()+1, s.totalShards())
}

func (s *shardFlag) Set(v string) error {
	if v == "" {
		s.current = 0
		s.total = 1
		return nil
	}

	parts := strings.SplitN(v, "/", 2)
	if len(parts) != 2 {
		return errInvalidShard
	}

	var err error
	s.current, err = strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("could not parse current shard value: %s", err)
	}
	if s.current < 1 {
		return fmt.Errorf("current shard (%d) must be > 0", s.current)
	}

	s.total, err = strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("could not parse total shard value: %s", err)
	}
	if s.total < s.current {
		return fmt.Errorf("total shard count must be >= than current (%d < %d)", s.total, s.current)
	}

	// Zero-index the current shard.
	s.current--
	return nil
}
