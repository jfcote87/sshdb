// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mssql_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"io/ioutil"
	"net"
	"os"
	"testing"

	pgkmssql "github.com/denisenkom/go-mssqldb"
	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/mssql"
	"gopkg.in/yaml.v3"
)

func TestTunnelDriver(t *testing.T) {
	if mssql.TunnelDriver.Name() != "mssql" {
		t.Errorf("expected Tunneler.Name() = \"mssql\"; got %s", mssql.TunnelDriver.Name())
	}
	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()

	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		cancelfunc()
		return nil, errors.New("no connect")
	})

	connectorFail, err := mssql.TunnelDriver.OpenConnector(dialer, "odbc:=====")
	if err == nil {
		t.Errorf("connectorfail expected \"unexpected character error\"; got %v", err)
		return
	}
	_ = connectorFail

	connector, err := mssql.TunnelDriver.OpenConnector(dialer, "sqlserver://sa:mypass@localhost?database=master&connection+timeout=30")
	if err != nil {
		t.Errorf("open connector failed %v", err)
		return
	}
	_, err = connector.Connect(ctx)
	select {
	case <-ctx.Done():
		return
	default:
	}
	t.Errorf("expected context cancelled; got %v", err)

}

func TestSetSessionInitSQL(t *testing.T) {
	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		return nil, nil
	})

	dsn00 := "sqlserver://sa:mypass@localhost?database=master&connection+timeout=30"
	dsn01 := "sqlserver://sa:mypass@example.com?database=master&connection+timeout=30"
	mssql.SetSessionInitSQL(dsn00, "")
	mssql.SetSessionInitSQL(dsn01, "INIT")

	var connectors = make([]driver.Connector, 2)
	var err error
	connectors[0], err = mssql.TunnelDriver.OpenConnector(dialer, dsn00)
	if err != nil {
		t.Errorf("open connector failed %v", err)
		return
	}
	connectors[1], err = mssql.TunnelDriver.OpenConnector(dialer, dsn01)
	if err != nil {
		t.Errorf("open connector failed %v", err)
		return
	}
	expectedValues := []string{"", "INIT"}
	for i, cx := range connectors {
		switch c := cx.(type) {
		case *pgkmssql.Connector:
			if c.SessionInitSQL != expectedValues[i] {
				t.Errorf("expected dsn0%d/connector[%d] to have SessionInitSQl = %q; got %s", i, i, expectedValues[i], c.SessionInitSQL)
			}
		default:
			t.Error("expected connector01 to be an mssql.Connector")
		}
	}

}

const testEnvName = "SSHDB_CONFIG_YAML_TEST_MSSQL"

func TestDriver_live(t *testing.T) {
	fn, ok := os.LookupEnv(testEnvName)
	if !ok {
		t.Skipf("test connection skipped, %s not found", testEnvName)
		return
	}
	buff, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Errorf("unable to open %s %v", fn, err)
		return
	}
	var cfg sshdb.Config
	if err := yaml.Unmarshal(buff, &cfg); err != nil {
		t.Errorf("%s unmarshal yaml %v", fn, err)
		return
	}
	dbids := cfg.DBList()
	dbs, err := cfg.OpenDBs(mssql.TunnelDriver)
	if err != nil {
		t.Errorf("opendbs failed: %v", err)
		return
	}
	for i := range dbs {
		if err := dbs[i].Ping(); err != nil {
			t.Errorf("%s - %v", dbids[i], err)
		}
	}
}
