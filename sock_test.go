// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || illumos
// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris illumos

package sshdb_test

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	unixSocket = fmt.Sprintf("/tmp/sshdb_%d.sock", time.Now().UnixNano())
	// Run tests
	exitVal := m.Run()
	// Exit with exit value from tests
	os.Exit(exitVal)
}
