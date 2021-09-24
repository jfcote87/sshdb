// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pgxv4 provides for ssh postgres connections via
// the "github.com/jackc/pgx/v4"
package pgxv4

import (
	"database/sql/driver"
	"fmt"
	"sync"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/jfcote87/sshdb"
)

// Opener used to register an ssh tunnel for postgres
var Opener sshdb.ConnectorOpener = opener("postgres_pgxv4")

var configFunc ConfigFunc
var mConfigFunc sync.Mutex

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

// NewConnector returns a new database/sql/driver connector
func (o opener) NewConnector(df sshdb.Dialer, dsn string) (driver.Connector, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	mConfigFunc.Lock()
	cf := configFunc
	mConfigFunc.Unlock()
	if err := cf.edit(cfg); err != nil {
		return nil, err
	}
	cfg.DialFunc = pgconn.DialFunc(df.DialContext)
	configName := stdlib.RegisterConnConfig(cfg)
	d, ok := stdlib.GetDefaultDriver().(*stdlib.Driver)
	if !ok {
		return nil, fmt.Errorf("expeect stdlib *driver")
	}
	return d.OpenConnector(configName)
}

type opener string

func (o opener) Name() string {
	return string(o)
}
