// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package mysql provide for mysql connections via the sshdb package
package mysql

import (
	"context"
	"database/sql/driver"
	"encoding/base64"
	"fmt"
	"net"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jfcote87/sshdb"
)

// TunnelDriver allows mysql connection via an sshdb tunnel.
var TunnelDriver sshdb.Driver = tunnelDriver("mysql")

// OpenConnector uses passed dialer to create a connection to the mssql database defined by the dsn variable.
func (tun tunnelDriver) OpenConnector(dialer sshdb.Dialer, dsn string) (driver.Connector, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	oriNet := cfg.Net
	// create random string for registration name
	cfg.Net = base64.RawStdEncoding.EncodeToString([]byte(fmt.Sprintf("tun_%d", time.Now().UnixNano())))

	mysql.RegisterDialContext(cfg.Net, func(ctx context.Context, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, oriNet, addr)
	})
	return mysql.NewConnector(cfg)
}

type tunnelDriver string

func (tun tunnelDriver) Name() string {
	return string(tun)
}
