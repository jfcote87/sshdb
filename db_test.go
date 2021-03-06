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

type Driver struct{}

func (d *Driver) OpenConnector(dsn string) (driver.Connector, error) {
	return nil, nil
}

func (d *Driver) Open(dsn string) (driver.Conn, error) {
	cn, err := d.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return cn.Connect(context.Background())
}

type Conn struct {
	net.Conn
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
	return c.driver
}

func (c *Conn) Ping(ctx context.Context) error {
	deadlines, _ := ctx.Value(deadlineKey).(string)
	if deadlines > "" {
		testDriverIgnoreDeadline = (deadlines == "ignore")
		err00 := c.SetDeadline(time.Now())
		err01 := c.SetReadDeadline(time.Now())
		err02 := c.SetWriteDeadline(time.Now())
		return errlist{err00, err01, err02}
	}
	buff := []byte("0123456789012345678901234567890123456789012345678901234567890123ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/")
	for i := 0; i < 5000; i++ {
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

// testDriver used to register an ssh tunnel for mssql
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

var testDriverIgnoreDeadline bool

type tunDriver string

func (tun tunDriver) IgnoreDeadlineError() bool {
	return testDriverIgnoreDeadline
}

func (tun tunDriver) Name() string {
	return string(tun)
}
