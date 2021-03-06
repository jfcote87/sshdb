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
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Tunnel is an ssh client connection that handles db
// connection through a remote host.
type Tunnel interface {
	driver.DriverContext
	driver.Driver
	io.Closer
}

// New returns a Tunnel based upon the ssh clientConfig for creating new connectors/connections
// via an ssh client connection.  The tunnel can host multiple db connections to different
// database servers. The tunnelDriver is a sshdb.Driver for a specific database type. For included
// implementations (mysql, mssql, pgx and pgx4) use <package name>.TunnelDriver.
// remoteHostPort defines the remote ssh server address and must be in the form "host:port",
// "host%zone:port", [host]:port" or "[host%zone]:port".  See func net.Dial for a more
// detailed description of the hostport format.
func New(tunnelDriver Driver, clientConfig *ssh.ClientConfig, remoteHostPort string) (Tunnel, error) {
	if clientConfig == nil {
		return nil, errors.New("clientConfig may not be nil")
	}
	if strings.Trim(remoteHostPort, " ") == "" {
		return nil, errors.New("remoteAddr may not be empty")
	}
	if _, _, err := net.SplitHostPort(remoteHostPort); err != nil {
		return nil, fmt.Errorf("invalid address - %w", err)
	}
	if tunnelDriver == nil {
		return nil, errors.New("tunnelDriver may not be nil")
	}
	resetChan := make(chan struct{})
	close(resetChan) // close to prevent reset calls prior to client
	return &tunnel{
		cfg:          clientConfig,
		addr:         remoteHostPort,
		tunnelDriver: tunnelDriver,
		connectors:   make(map[string]*connector),
		sshconns:     make(map[*sshConn]bool),
		resetChan:    resetChan,
	}, nil
}

// Driver used to create connectors for the underlying database driver.
type Driver interface {
	OpenConnector(dialer Dialer, dsn string) (driver.Connector, error)
	Name() string
}

// connector is returned by the dialer.OpenConnector func and wraps the
// underlying db connector to allow tracking of each connection.
type connector struct {
	driver.Connector
	tunnel Tunnel // *tunnel
}

// Driver ensures that the type connector fulfills the driver.Connector interface,
func (c *connector) Driver() driver.Driver {
	return c.tunnel
}

type sshConn struct {
	tunnel *tunnel
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
	if ide, ok := sc.tunnel.tunnelDriver.(ignoreDeadlineError); ok && ide.IgnoreDeadlineError() {
		return nil
	}
	return sc.Conn.SetDeadline(tm)
}

// SetReadDeadline is not implemented by the ssh tcp connection.  If
// the tunnel Driver implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetReadDeadline(tm time.Time) error {
	if ide, ok := sc.tunnel.tunnelDriver.(ignoreDeadlineError); ok && ide.IgnoreDeadlineError() {
		return nil
	}
	return sc.Conn.SetReadDeadline(tm)
}

// SetDeadline is not implemented by the ssh tcp connection.  If
// the tunnel Driver implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetWriteDeadline(tm time.Time) error {
	if ide, ok := sc.tunnel.tunnelDriver.(ignoreDeadlineError); ok && ide.IgnoreDeadlineError() {
		return nil
	}
	return sc.Conn.SetReadDeadline(tm)
}

type ignoreDeadlineError interface {
	IgnoreDeadlineError() bool
}

// tunnel manages an ssh client connections and creates
// and tracks db connections made through the client.
type tunnel struct {
	cfg          *ssh.ClientConfig
	addr         string // format <hostname>:<port>
	tunnelDriver Driver
	connectors   map[string]*connector // map of dsn to connector
	mConn        sync.Mutex            // protects connectors, sshconns and client

	sshconns  map[*sshConn]bool // initialized on dialcontext
	client    *ssh.Client
	resetChan chan struct{} // closed at reset
	m         sync.Mutex    //prootects sshconns, client and resetChan
}

// OpenConnector fulfills the driver DriverContext interface and returns a new
// db connection via the ssh client connection.  The dataSourceName should follow
// rules of the base database and must create the connection as if connecting from
// the remote ssh connection.
func (tun *tunnel) OpenConnector(dataSourceName string) (driver.Connector, error) {
	tun.mConn.Lock()
	defer tun.mConn.Unlock()
	if connector, ok := tun.connectors[dataSourceName]; ok {
		return connector, nil
	}
	dbconnector, err := tun.tunnelDriver.OpenConnector(Dialer(tun), dataSourceName)
	if err != nil {
		return nil, err
	}
	c := &connector{
		Connector: dbconnector,
		tunnel:    tun,
	}
	tun.connectors[dataSourceName] = c
	return c, nil
}

// Open fulfills the driver.Driver interface.
func (tun *tunnel) Open(dsn string) (driver.Conn, error) {
	connector, err := tun.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return connector.Connect(context.Background())
}

// Close safely resets the  tunnel. If calling func has already
// locked tunnel.m, it should call reset directly.
func (tun *tunnel) Close() error {
	tun.m.Lock()
	err := tun.reset()
	tun.m.Unlock()
	return err
}

// reset closes the tunnel's client connection and closes
// all existing db connections.  Routines must obtain a lock
// on tunnel.m prior to calling.  After reset, the tunnel can
// still create new connections and  existing connectors are
// valid.
func (tun *tunnel) reset() error {
	select {
	case <-tun.resetChan:
		return nil
	default:
	}
	// close channel to prevent duplicate resets
	close(tun.resetChan)
	for k := range tun.sshconns {
		k.Conn.Close()
	}
	tun.sshconns = make(map[*sshConn]bool)
	if tun.client == nil {
		return nil
	}
	return tun.client.Close()
}

// DialContext opens a network connection over the ssh client    First
// checking whether an underlying ssh client connection is available and initiates
// the client connection if needed.  A new ssh client connection to the database
// is attempted to the host and port defined by addr.
func (tun *tunnel) DialContext(ctx context.Context, net, addr string) (net.Conn, error) {
	// lock sd for the duration of funcs
	tun.m.Lock()
	defer tun.m.Unlock()

	ctxchan := ctx.Done()
	select {
	case <-ctxchan:
		return nil, ctx.Err()
	case <-tun.resetChan:
		// create tunnel ssh client connection
		cl, err := ssh.Dial("tcp", tun.addr, tun.cfg)
		if err != nil {
			return nil, err
		}
		select {
		case <-ctxchan:
			cl.Close() // context cancelled, close new client connection
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
	return tun.newNetConn(addr)
}

func (tun *tunnel) newNetConn(addr string) (*sshConn, error) {
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
// managed by the tunnel.
func (tun *tunnel) ConnCount() int {
	tun.m.Lock()
	cnt := len(tun.sshconns)
	tun.m.Unlock()
	return cnt
}

// DBName returns name of underlying driver.
func (tun *tunnel) DBName() string {
	return tun.tunnelDriver.Name()
}

// Dialer creates a net.Conn via the tunnel's ssh client.
type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// DialerFunc allows a func to fulfill the Dialer interface.
type DialerFunc func(context.Context, string, string) (net.Conn, error)

// DialContext calls the underlying dialerfunc.
func (d DialerFunc) DialContext(ctx context.Context, net, addr string) (net.Conn, error) {
	return d(ctx, net, addr)
}
