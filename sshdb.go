// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sshdb provides golang.org/x/crypto/ssh client connections
// to tunnel database connection on a remote server
package sshdb

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Tunnel is an ssh client connection that handles db
// connection through a remote host
type Tunnel interface {
	driver.DriverContext
	driver.Driver
	io.Closer
}

// New returns a Tunnel based upon the ssh clientConfig for creating new connectors/connections
// via an ssh client connection.  The tunnel can host multiple db connections event to different
// database servers. The opener for included packages is <package name>.Opener. remoteAddr
// is the ip address of the remote host.
func New(opener ConnectorOpener, clientConfig *ssh.ClientConfig, remoteAddr string) (Tunnel, error) {
	if clientConfig == nil {
		return nil, errors.New("clientConfig may not be nil")
	}
	if remoteAddr == "" {
		return nil, errors.New("remoteAddr may not be empty")
	}
	if opener == nil {
		return nil, errors.New("opener may not be nil")
	}
	resetChan := make(chan struct{})
	close(resetChan) // close to prevent reset calls prior to client
	return &tunnel{
		cfg:         clientConfig,
		addr:        remoteAddr,
		connCreator: opener,
		connectors:  make(map[string]*connector),
		sshconns:    make(map[*sshConn]bool),
		resetChan:   resetChan,
	}, nil
}

// ConnectorOpener used to create connectors for the underlying database driver.
type ConnectorOpener interface {
	NewConnector(dialer Dialer, dsn string) (driver.Connector, error)
	Name() string
}

// connector is returned by the dialer.OpenConnector func and wraps the
// underlying db connector to allow tracking of each connection.
type connector struct {
	driver.Connector
	tunnel Tunnel // *tunnel
}

// Driver fullfiils the Connector interface,
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
	tunnel.m.Unlock()
	if len(tunnel.sshconns) > 1 {
		delete(tunnel.sshconns, sc)
		return sc.Conn.Close()
	}
	return tunnel.reset()
}

// SetDeadline is not implemented by the ssh tcp connection.  If
// the connCreator implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetDeadline(tm time.Time) error {
	if ide, ok := sc.tunnel.connCreator.(ignoreDeadlineError); ok && ide.IgnoreDeadlineError() {
		return nil
	}
	return sc.Conn.SetDeadline(tm)
}

// SetReadDeadline is not implemented by the ssh tcp connection.  If
// the connCreator implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetReadDeadline(tm time.Time) error {
	if ide, ok := sc.tunnel.connCreator.(ignoreDeadlineError); ok && ide.IgnoreDeadlineError() {
		return nil
	}
	return sc.Conn.SetReadDeadline(tm)
}

// SetDeadline is not implemented by the ssh tcp connection.  If
// the connCreator implements ingnoreDeadlineError then a nil is
// returned rather than a not implemented error.
func (sc *sshConn) SetWriteDeadline(tm time.Time) error {
	if ide, ok := sc.tunnel.connCreator.(ignoreDeadlineError); ok && ide.IgnoreDeadlineError() {
		return nil
	}
	return sc.Conn.SetReadDeadline(tm)
}

type ignoreDeadlineError interface {
	IgnoreDeadlineError() bool
}

// tunnel manages an ssh client connections and
// creates and tracks db connections made through the client
type tunnel struct {
	cfg         *ssh.ClientConfig
	addr        string // format <hostname>:<port>
	connCreator ConnectorOpener
	connectors  map[string]*connector // map of dsn to connector
	mConn       sync.Mutex            // protects connectors

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
	dbconnector, err := tun.connCreator.NewConnector(Dialer(tun), dataSourceName)
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

// Open fulfills the driver.Driver interface
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
			cl.Close()
			return nil, ctx.Err()
		default:
			tun.client = cl
		}
		chx := make(chan struct{})
		tun.resetChan = chx
		go func() {
			_ = cl.Wait()
			select {
			case <-chx:
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
// managed by the tunnel
func (tun *tunnel) ConnCount() int {
	tun.m.Lock()
	cnt := len(tun.sshconns)
	tun.m.Unlock()
	return cnt
}

// DBName returns name of underlying driver
func (tun *tunnel) DBName() string {
	return tun.connCreator.Name()
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
