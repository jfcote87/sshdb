// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package oracle provide for mysql connections via the sshdb package
package oracle

import (
	"database/sql/driver"
	"fmt"

	"github.com/jfcote87/sshdb"
	ora "github.com/sijms/go-ora/v2"
)

const driverName = "oracle"

// TunnelDriver creates mysql connectors via that connect via sshdb tunnels
var TunnelDriver sshdb.Driver = tunnelDriver(driverName)

// OpenConnector returns a new oracle connector that uses the dialer to open ssh channel connections
// as the underlying network connections
func (tun tunnelDriver) OpenConnector(dialer sshdb.Dialer, dsn string) (driver.Connector, error) {

	oc := new(ora.OracleDriver)

	connector, err := oc.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}

	oConnector, ok := connector.(*ora.OracleConnector)
	if !ok {
		fmt.Println(err)
	}
	oConnector.Dialer(dialer)
	return oConnector, nil
}

type tunnelDriver string

func (tun tunnelDriver) Name() string {
	return string(tun)
}
