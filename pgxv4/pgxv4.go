// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pgxv4 provides for ssh postgres connections via
// the "github.com/jackc/pgx/v4"
package pgxv4

import (
	"database/sql/driver"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/jfcote87/sshdb"
)

type tunnelDriver string

func (tun tunnelDriver) Name() string {
	return string(tun)
}

// TunnelDriver used to register an ssh tunnel for postgres
var TunnelDriver sshdb.Driver = tunnelDriver("postgres_pgxv4")

// OpenConnector returns a new database/sql/driver connector
func (tun tunnelDriver) OpenConnector(df sshdb.Dialer, dsn string) (driver.Connector, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	cfg.DialFunc = pgconn.DialFunc(df.DialContext)
	configName := stdlib.RegisterConnConfig(cfg)
	// GetDefaultDriver always returns non-nil driver.DriverContext
	return stdlib.GetDefaultDriver().(driver.DriverContext).OpenConnector(configName)

}
