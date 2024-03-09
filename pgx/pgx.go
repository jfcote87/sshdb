// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pgx provides for ssh postgres connections via
// the github.com/jackc/pgx package
package pgx

import (
	"context"
	"database/sql/driver"
	"net"
	"sync"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	"github.com/jfcote87/sshdb"
)

var configFunc ConfigFunc
var mConfigFunc sync.Mutex

// TunnelDriver is used to register the postgres sql driver pgx version3
var TunnelDriver sshdb.Driver = tunnelDriver("pgx")

// ConfigFunc updates fields in a ConnConfig after
// it is created by parsing the passed dsn.
type ConfigFunc func(*pgx.ConnConfig) error

// SetConfigEdit links a ConfigFunc to a dsn string.  When creating
// a new connector, the dsn will be used to match the ConfigFunc.
func SetConfigEdit(cf ConfigFunc) {
	mConfigFunc.Lock()
	configFunc = cf
	mConfigFunc.Unlock()
}

func (c ConfigFunc) edit(cc *pgx.ConnConfig) error {
	if c == nil {
		return nil
	}
	return c(cc)
}

// OpenConnector returns a connector based upon the DialFunc
func (tun tunnelDriver) OpenConnector(df sshdb.Dialer, dsn string) (driver.Connector, error) {

	cfg, err := pgx.ParseConnectionString(dsn)
	if err != nil {
		return nil, err
	}
	mConfigFunc.Lock()
	cf := configFunc
	mConfigFunc.Unlock()
	if err := cf.edit(&cfg); err != nil {
		return nil, err
	}

	cfg.Dial = func(network, addr string) (net.Conn, error) {
		return df.DialContext(context.Background(), network, addr)
	}
	dc := &stdlib.DriverConfig{
		ConnConfig: cfg,
	}

	stdlib.RegisterDriverConfig(dc)
	nm := dc.ConnectionString(dsn)
	return &connector{
		driver:   stdlib.GetDefaultDriver(),
		nm:       nm,
		connConf: cfg,
	}, nil
}

type connector struct {
	driver   *stdlib.Driver
	nm       string
	connConf pgx.ConnConfig
}

func (c *connector) Driver() driver.Driver {
	return c.driver
}

func (c *connector) Connect(_ context.Context) (driver.Conn, error) {
	return c.driver.Open(c.nm)
}

func (c *connector) GetConnConfig() pgx.ConnConfig {
	return c.connConf
}

type tunnelDriver string

func (tun tunnelDriver) Name() string {
	return string(tun)
}
