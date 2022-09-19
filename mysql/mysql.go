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

func init() {
	// register name with sshdb
	sshdb.RegisterDriver(driverName, TunnelDriver)
}

const driverName = "mysql"

// TunnelDriver creates mysql connectors via that connect via sshdb tunnels
var TunnelDriver sshdb.Driver = tunnelDriver(driverName)

// OpenConnector returns a new mssql connector that uses the dialer to open ssh channel connections
// as the underlying network connections
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
