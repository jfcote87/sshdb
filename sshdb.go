// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sshdb provides database connections to tunnel
// through an ssh connection to a remove server
package sshdb

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// A Driver creates connectors that use the passed dialer
// to make connections via the ssh tunnel.
//
// This package includes drivers for mysql, mssql, postgress (v3 and v4).
type Driver interface {
	OpenConnector(dialer Dialer, dsn string) (driver.Connector, error)
	Name() string
}

// Dialer creates a net.Conn via the tunnel's ssh client
type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// DialerFunc allows a func to fulfill the Dialer interface.
type DialerFunc func(context.Context, string, string) (net.Conn, error)

// DialContext calls the underlying dialerfunc.
func (d DialerFunc) DialContext(ctx context.Context, net, addr string) (net.Conn, error) {
	return d(ctx, net, addr)
}

// New returns a Tunnel based upon the ssh clientConfig for creating new connectors/connections
// via an ssh client connection.  The tunnel can host multiple db connections to different
// database servers. The tunnelDriver is a sshdb.Driver for a specific database type. For included
// implementations (mysql, mssql, pgx and pgx4) use <package name>.TunnelDriver.
// remoteHostPort defines the remoteanother ssh server address and must be in the form "host:port",
// "host%zone:port", [host]:port" or "[host%zone]:port".  See func net.Dial for a more
// detailed description of the hostport format.
func New(clientConfig *ssh.ClientConfig, remoteHostPort string) (*Tunnel, error) {
	if clientConfig == nil {
		return nil, errors.New("clientConfig may not be nil")
	}
	if strings.Trim(remoteHostPort, " ") == "" {
		return nil, errors.New("remoteAddr may not be empty")
	}
	if _, _, err := net.SplitHostPort(remoteHostPort); err != nil {
		return nil, fmt.Errorf("invalid address - %w", err)
	}

	resetChan := make(chan struct{})
	close(resetChan) // close to prevent reset calls prior to client initialization

	return &Tunnel{
		cfg:        clientConfig,
		addr:       remoteHostPort,
		connectors: make(map[string]driver.Connector),
		sshconns:   make(map[*sshConn]bool),
		resetChan:  resetChan,
	}, nil
}

// Tunnel manages an ssh client connections and
// creates and tracks db connections made through the client
type Tunnel struct {
	cfg                      *ssh.ClientConfig
	addr                     string                      // format <hostname>:<port>
	connectors               map[string]driver.Connector // map of dsn to connector
	ignoreSetDeadlineRequest bool
	mConn                    sync.Mutex // protects connectors and ignoreDeadlineError

	sshconns  map[*sshConn]bool // initialized on dialcontext
	client    *ssh.Client
	resetChan chan struct{} // closed at reset
	m         sync.Mutex    //protects sshconns, client and resetChan
}

// IgnoreSetDeadlineRequest exists because the ssh client package does not support
// deadlines and returns an error if attempting to set a deadline.  If existing
// code contains setdeadline calls, pass true to this functions, and the tunnel
// ignore deadline requests.
func (tun *Tunnel) IgnoreSetDeadlineRequest(val bool) {
	tun.mConn.Lock()
	tun.ignoreSetDeadlineRequest = val
	tun.mConn.Unlock()
}

// OpenConnector fulfills the driver DriverContext interface and returns a new
// db connection via the ssh client connection.  The dataSourceName should follow
// rules of the base database and must create the connection as if connecting from
// the remote ssh connection.
func (tun *Tunnel) OpenConnector(tunnelDriver Driver, dataSourceName string) (driver.Connector, error) {
	tun.mConn.Lock()
	defer tun.mConn.Unlock()
	connectorName := tunnelDriver.Name() + ":" + dataSourceName
	if connector, ok := tun.connectors[connectorName]; ok {
		return connector, nil
	}
	dbconnector, err := tunnelDriver.OpenConnector(DialerFunc(tun.DialContext), dataSourceName)
	if err != nil {
		return nil, err
	}
	tun.connectors[connectorName] = dbconnector
	return dbconnector, nil
}

// Close safely resets the  tunnel. If calling func has already
// locked tunnel.m, it should call reset directly.
func (tun *Tunnel) Close() error {
	tun.m.Lock()
	err := tun.reset()
	tun.m.Unlock()
	return err
}

// DialContext creates an ssh client connection to the addr.  sshdb drivers must use this
// func when creating driver.Connectors.  You may use this func establish "raw" connections
// to a remote service.
func (tun *Tunnel) DialContext(ctx context.Context, net, addr string) (net.Conn, error) {
	// lock sd for the duration
	tun.m.Lock()
	defer tun.m.Unlock()

	ctxchan := ctx.Done()
	select {
	// check for timeout or cancel of ctx
	case <-ctxchan:
		return nil, ctx.Err()

	// if tunnel is not open create new tunnel
	case <-tun.resetChan:
		// create tunnel ssh client connection
		cl, err := ssh.Dial("tcp", tun.addr, tun.cfg)
		if err != nil {
			return nil, err
		}
		select {
		case <-ctxchan:
			cl.Close() // if context cancelled, close new client connection
			return nil, ctx.Err()
		default:
		}
		tun.client = cl
		clientResetChannel := make(chan struct{})
		tun.resetChan = clientResetChannel
		go func() {
			// if client connection close (network error)
			// reset channel to close all db connections
			_ = cl.Wait()
			select {
			case <-clientResetChannel:
				return
			default:
				tun.Close()
			}
		}()

	default:
	}
	// make connection
	return tun.getNetConn(addr)
}

// reset closes the tunnel's client connection and closes
// all existing db connections.  Routines must obtain a lock
// on tunnel.m prior to calling.  After reset, the tunnel can
// still create new connections and  existing connectors are
// valid.
func (tun *Tunnel) reset() error {
	select {
	case <-tun.resetChan:
		// do nothing
	default:
		// close channel to prevent duplicate resets
		close(tun.resetChan)
		for k := range tun.sshconns {
			k.Conn.Close()
		}
		tun.sshconns = make(map[*sshConn]bool)
		if tun.client != nil {
			return tun.client.Close()
		}
	}
	return nil
}

// getNetConn create a client connection through the tunnel
func (tun *Tunnel) getNetConn(addr string) (net.Conn, error) {
	network := "tcp"
	if len(addr) > 0 && addr[0] == '/' {
		network = "unix"
	}
	conn, err := tun.client.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	sshconn := &sshConn{
		tunnel: tun,
		Conn:   conn,
	}
	tun.sshconns[sshconn] = true
	return sshconn, nil
}

// ConnCount returns number of active db connections
// managed by the tunnel
func (tun *Tunnel) ConnCount() int {
	tun.m.Lock()
	cnt := len(tun.sshconns)
	tun.m.Unlock()
	return cnt
}

// handleDeadline uses the ignoreDeadlineError to determine whether to
// handle a setdeadline call on a sshConn
func (tun *Tunnel) handleDeadline(tm time.Time, setDeadline func(time.Time) error) error {
	var err error
	tun.mConn.Lock()
	if !tun.ignoreSetDeadlineRequest {
		err = setDeadline(tm)
	}
	tun.mConn.Unlock()
	return err
}

type sshConn struct {
	tunnel *Tunnel
	net.Conn
}

// Close resets tunnel if last connection other wise
// closes connection and updates tunnel ssh connections
// map.
func (sc *sshConn) Close() error {
	tunnel := sc.tunnel
	tunnel.m.Lock()
	defer tunnel.m.Unlock()
	if len(tunnel.sshconns) > 1 {
		delete(tunnel.sshconns, sc)
		return sc.Conn.Close()
	}
	return tunnel.reset()
}

// SetDeadline is not implemented by the ssh tcp connection.  If
// the tunnel Driver implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetDeadline(tm time.Time) error {
	return sc.tunnel.handleDeadline(tm, sc.Conn.SetDeadline)
}

// SetReadDeadline is not implemented by the ssh tcp connection.  If
// the tunnel Driver implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetReadDeadline(tm time.Time) error {
	return sc.tunnel.handleDeadline(tm, sc.Conn.SetReadDeadline)
}

// SetDeadline is not implemented by the ssh tcp connection.  If
// the tunnel Driver implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetWriteDeadline(tm time.Time) error {
	return sc.tunnel.handleDeadline(tm, sc.Conn.SetWriteDeadline)
}
