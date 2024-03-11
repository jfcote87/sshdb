// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jfcote87/sshdb"
)

type Driver struct {
	Connector driver.Connector
}

func (d *Driver) OpenConnector(dsn string) (driver.Connector, error) {
	if strings.HasPrefix(dsn, "bad") || strings.HasPrefix(dsn, "ERR") {
		return nil, fmt.Errorf("%s", dsn)
	}
	return d.Connector, nil
}

func (d *Driver) Open(dsn string) (driver.Conn, error) {
	_, err := d.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return &Conn{}, nil
}

type Conn struct {
	net.Conn
}

func (c *Conn) Close() error {
	if c != nil && c.Conn != nil {
		return c.Conn.Close()
	}
	return nil
}

func (c *Conn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("prepare not implemented")
}

func (c *Conn) Begin() (driver.Tx, error) {
	return nil, errors.New("begin tx not implemented")
}

type Connector struct {
	sshdb.Dialer
	addr   string
	driver driver.Driver
}

func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Dialer.DialContext(ctx, "", c.addr)
	if err != nil {
		return nil, err
	}
	return &Conn{conn}, nil
}

func (c *Connector) Driver() driver.Driver {
	return &Driver{
		Connector: c,
	}
}

func (c *Conn) Ping(ctx context.Context) error {
	deadlineFunc, _ := ctx.Value(deadlineKey).(func())
	if deadlineFunc != nil {
		deadlineFunc()
		err00 := c.SetDeadline(time.Now())
		err01 := c.SetReadDeadline(time.Now())
		err02 := c.SetWriteDeadline(time.Now())
		return errlist{err00, err01, err02}
	}
	buff := []byte("0123456789012345678901234567890123456789012345678901234567890123ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")
	for i := 0; i < 10000; i++ {
		if _, err := c.Write(buff); err != nil {
			return err
		}
		if _, err := c.Read(buff); err != nil {
			return err
		}
	}
	return nil
}

type errlist []error

func (el errlist) Error() string {
	b := strings.Builder{}
	for _, err := range el {
		b.WriteString(fmt.Sprintf("%#v\n", err))
	}
	return b.String()
}

// testDriver used to register an ssh tunnel
var testDriver = tunDriver("sshdbtest")

// New returns a new database/sql/driver connector
func (tun tunDriver) OpenConnector(dialer sshdb.Dialer, dsn string) (driver.Connector, error) {
	if strings.HasPrefix(dsn, "ERR") {
		return nil, fmt.Errorf("invalid dsn %s", dsn)
	}
	df := sshdb.DialerFunc(dialer.DialContext) // just to test DialerFunc
	return &Connector{
		Dialer: df,
		addr:   dsn,
		driver: &Driver{},
	}, nil
}

type tunDriver string

func (tun tunDriver) Name() string {
	return string(tun)
}
