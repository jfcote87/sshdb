// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mssql provides for mssql connection via the sshdb package.
package mssql

import (
	"database/sql/driver"
	"sync"

	mssql "github.com/denisenkom/go-mssqldb"
	"github.com/jfcote87/sshdb"
)

// TunnelDriver allows mssql connection via an sshdb tunnel.
var TunnelDriver sshdb.Driver = tunnelDriver("mssql")

// OpenConnector uses passed dialer to create a connection to the mssql database defined by the dsn variable.
func (tun tunnelDriver) OpenConnector(dialer sshdb.Dialer, dsn string) (driver.Connector, error) {
	connector, err := mssql.NewConnector(dsn)
	if err != nil {
		return nil, err
	}

	connector.Dialer = mssql.Dialer(dialer)
	mMap.Lock()
	connector.SessionInitSQL = mapSessionInitSQL[dsn]
	mMap.Unlock()
	return connector, nil
}

type tunnelDriver string

func (tun tunnelDriver) Name() string {
	return string(tun)
}

var mapSessionInitSQL = make(map[string]string)
var mMap sync.Mutex

// SetSessionInitSQL will add the sql to the connector's SessionInitSQL
// value whenever the dsn values match.
func SetSessionInitSQL(dsn, sql string) {
	mMap.Lock()
	defer mMap.Unlock()
	if sql == "" {
		delete(mapSessionInitSQL, dsn)
		return
	}
	mapSessionInitSQL[dsn] = sql
}
