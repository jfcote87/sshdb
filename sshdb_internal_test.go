// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestDialContext checks that tun.DialContext handles context
// cancellation
func TestDialContext(t *testing.T) {
	tun := Tunnel{}
	ctx, cancelfunc := context.WithCancel(context.Background())
	cancelfunc()
	cx, err := tun.DialContext(ctx, "", "noaddr:3600")
	if err == nil {
		cx.Close()
	}
	if err != context.Canceled {
		t.Errorf("expected context.CancelFunc; got %v", err)
	}
}

type ConnectionCounter interface {
	ConnCount() int
}

// ConnectionCount returns the number of open connections
// in a tunnel. This is used only for testing.
func ConnectionCount(driverCtx interface{}) (int, error) {
	tun, ok := driverCtx.(ConnectionCounter)
	if ok {
		return tun.ConnCount(), nil
	}
	return 0, errors.New("expected a sshdb.tunnel")
}

// CloseClient closes mimics a network error closing
// the tunnel's client connection.  Tests that
// tunnel is reset.
func CloseClient(tun *Tunnel) error {
	if tun == nil {
		return errors.New("nil *Tunnel")
	}
	tun.m.Lock()
	ch := tun.resetChan
	tun.m.Unlock()
	tun.client.Close()
	select {
	case <-ch: //
		tun.m.Lock()
		tun.m.Unlock()
		if cnt, _ := ConnectionCount(tun); cnt != 0 {
			return fmt.Errorf("expected 0 connections; found %d", cnt)
		}
		//
		tun.m.Lock() // set channel for reset w nil client
		tun.resetChan = make(chan struct{})
		tun.m.Unlock()

		return nil
	case <-time.After(time.Second): // give it a second finish
	}
	return errors.New("timeout")

}
