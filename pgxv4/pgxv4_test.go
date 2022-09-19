// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pgx provides for ssh postgres connections via
// the github.com/jackc/pgx package

package pgxv4_test

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/internal"
	"github.com/jfcote87/sshdb/pgxv4"
)

func TestTunnelDriver(t *testing.T) {
	if pgxv4.TunnelDriver.Name() != "postgres_pgxv4" {
		t.Errorf("expected TunnelDriver.Name() = \"postgres_pgxv4\"; got %s", pgxv4.TunnelDriver.Name())
	}
	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()

	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		cancelfunc()
		return nil, errors.New("no connect")
	})
	connectorFail, err := pgxv4.TunnelDriver.OpenConnector(dialer, "applic2.3.4 dbname=mydb")
	if err != nil {
		t.Errorf("connectorfail expected \"unexpected character error\"; got %v", err)
		return
	}
	_ = connectorFail
	connector, err := pgxv4.TunnelDriver.OpenConnector(dialer, "postgres://jack:secret@10.52.32.93:5432/mydb?sslmode=verify-ca")
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

func TestConfigFunc(t *testing.T) {
	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		return nil, errors.New("no connect")
	})
	//var changedUserName = "CHANGEDUSER"
	dsn00 := "application_name=pgxtest00 search_path=admin user=username password=password host=1.2.3.4 dbname=mydb00"
	dsn01 := "postgres://{user}&pwd&>/abc"
	dsn02 := "======"

	_, err := pgxv4.TunnelDriver.OpenConnector(dialer, dsn00)
	if err != nil {
		t.Errorf("dsn00 expected successful open; got %v", err)
		return
	}

	if _, err = pgxv4.TunnelDriver.OpenConnector(dialer, dsn01); err == nil {
		t.Errorf("dsn01 expected newconnector error; got <nil>")

	}
	if _, err := pgxv4.TunnelDriver.OpenConnector(dialer, dsn02); err == nil {
		t.Errorf("dsn02 expected newconnector error; got <nil>")
	}
}

const testEnvName = "SSHDB_CONFIG_YAML_TEST_PGXV4"

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
