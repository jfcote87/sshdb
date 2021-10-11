// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pgx provides for ssh postgres connections via
// the github.com/jackc/pgx package

package pgxv4_test

import (
	"context"
	"errors"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/pgxv4"
	"gopkg.in/yaml.v3"
)

func TestTunnelDriver(t *testing.T) {
	if pgxv4.TunnelDriver.Name() != "pgxv4" {
		t.Errorf("expected TunnelDriver.Name() = \"pgxv4\"; got %s", pgxv4.TunnelDriver.Name())
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
	var changedUserName = "CHANGEDUSER"
	dsn00 := "application_name=pgxtest00 search_path=admin user=username password=password host=1.2.3.4 dbname=mydb00"
	dsn01 := "application_name=pgxtest01 search_path=admin user=username password=password host=1.2.3.4 dbname=mydb01"
	dsn02 := "======"

	pgxv4.SetConfigEdit(func(cfg *pgx.ConnConfig) error {
		if cfg.Database == "mydb00" {
			cfg.User = changedUserName
			return nil
		}
		return errors.New("failure")
	})
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
	pgxv4.SetConfigEdit(nil)
}

const testEnvName = "SSHDB_CONFIG_YAML_TEST_PGXV4"

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
	dbs, err := cfg.OpenDBs(pgxv4.TunnelDriver)
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
