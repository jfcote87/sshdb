// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mssql_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"testing"

	pgkmssql "github.com/denisenkom/go-mssqldb"
	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/internal"
	"github.com/jfcote87/sshdb/mssql"
)

func TestOpener(t *testing.T) {
	if mssql.Opener.Name() != "mssql" {
		t.Errorf("expected ConnectorCreator.Name() = \"mssql\"; got %s", mssql.Opener.Name())
	}
	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()

	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		cancelfunc()
		return nil, errors.New("no connect")
	})

	connectorFail, err := mssql.Opener.NewConnector(dialer, "odbc:=====")
	if err == nil {
		t.Errorf("connectorfail expected \"unexpected character error\"; got %v", err)
		return
	}
	_ = connectorFail

	connector, err := mssql.Opener.NewConnector(dialer, "sqlserver://sa:mypass@localhost?database=master&connection+timeout=30")
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

	var connectors = make([]driver.Connector, 2, 2)
	var err error
	connectors[0], err = mssql.Opener.NewConnector(dialer, dsn00)
	if err != nil {
		t.Errorf("open connector failed %v", err)
		return
	}
	connectors[1], err = mssql.Opener.NewConnector(dialer, dsn01)
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

func TestOpener_live(t *testing.T) {
	db, err := internal.DBFromEnvSettings(mssql.Opener)
	if err != nil {
		if err == internal.ErrNoEnvVariable {
			t.Skip("test connection skipped, SSHDB_CLIENT_CONNECTION_MSSQL not found")
			return
		}
		t.Errorf("%v", err)
		return
	}
	if err = db.Ping(); err != nil {
		t.Errorf("ping failure %v", err)
	}
}
