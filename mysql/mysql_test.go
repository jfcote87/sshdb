// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mysql_test

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/jfcote87/sshdb"

	"github.com/jfcote87/sshdb/internal"
	"github.com/jfcote87/sshdb/mysql"
)

func TestTunnelDriver(t *testing.T) {

	if mysql.TunnelDriver.Name() != "mysql" {
		t.Errorf("expected TunnelDriver.Name() = \"mysql\"; got %s", mysql.TunnelDriver.Name())
	}

	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()
	cnt := 1
	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		cancelfunc()
		cnt++
		return nil, errors.New("no connect")
	})

	connectorFail, err := mysql.TunnelDriver.OpenConnector(dialer, "sa:passwordzzz(localhost:3306)schema")
	if err == nil {
		t.Errorf("connectorfail expected dsn error ; got %v", err)
		return
	}
	_ = connectorFail

	connector, err := mysql.TunnelDriver.OpenConnector(dialer, "sa:password@tcp(localhost:3306)/schema")
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

const testEnvName = "SSHDB_CONFIG_YAML_TEST_MYSQL"

func TestDriver_live(t *testing.T) {
	fn, ok := os.LookupEnv(testEnvName)
	if !ok {
		t.Skipf("test connection skipped, %s not found", testEnvName)
		return
	}
	cfg, err := internal.LoadTunnelConfig(fn)
	if err != nil {
		t.Errorf("load: %v", err)
		return
	}
	dbs, err := cfg.DatabaseMap()
	if err != nil {
		t.Errorf("open databases failed: %v", err)
		return
	}

	for nm, db := range dbs {
		defer db.Close()
		if err := db.Ping(); err != nil {
			t.Errorf("%s: ping %v", nm, err)
		}
		for _, qry := range cfg.Datasources[nm].Queries {
			if err := liveDBQuery(db, qry); err != nil {
				t.Errorf("%s: %s: %v", nm, qry, err)
			}
		}
	}
}

func liveDBQuery(db *sql.DB, qry string) error {
	rows, err := db.Query(qry)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {

	}
	return nil
}
