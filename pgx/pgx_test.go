// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package pgx_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/jackc/pgx"
	"github.com/jfcote87/sshdb"
	sshdbpgx "github.com/jfcote87/sshdb/pgx"
	"gopkg.in/yaml.v3"
)

func TestTunnelDriver(t *testing.T) {
	if sshdbpgx.TunnelDriver.Name() != "pgx" {
		t.Errorf("expected TunnelDriver.Name() = \"pgx\"; got %s", sshdbpgx.TunnelDriver.Name())
	}
	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()

	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		cancelfunc()
		return nil, errors.New("no connect")
	})
	connectorFail, err := sshdbpgx.TunnelDriver.OpenConnector(dialer, "host=256.634.63.346.3 port=a")
	if err == nil {
		t.Errorf("connectorfail expected \"unexpected character error\"; got %v", err)
		return
	}
	_ = connectorFail

	connector, err := sshdbpgx.TunnelDriver.OpenConnector(dialer, "application_name=pgxtest search_path=admin user=username password=password host=1.2.3.4 dbname=mydb")
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

type getConnConfig interface {
	GetConnConfig() pgx.ConnConfig
}

func TestConfigFunc(t *testing.T) {
	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		return nil, errors.New("no connect")
	})
	var changedUserName = "CHANGEDUSER"
	dsn00 := "application_name=pgxtest00 search_path=admin user=username password=password host=1.2.3.4 dbname=mydb00"
	dsn01 := "application_name=pgxtest01 search_path=admin user=username password=password host=1.2.3.4 dbname=mydb01"

	sshdbpgx.SetConfigEdit(func(cfg *pgx.ConnConfig) error {
		if cfg.Database == "mydb00" {
			cfg.User = changedUserName
			return nil
		}
		return errors.New("failure")
	})

	connector00, err := sshdbpgx.TunnelDriver.OpenConnector(dialer, dsn00)
	if err != nil {
		t.Errorf("expected successful open for dsn01; got %v", err)
		return
	}
	if _, ok := connector00.Driver().(driver.DriverContext); ok {
		t.Errorf("expected driver to notd be a DriverContext")
		return
	}

	gc, ok := connector00.(getConnConfig)
	if !ok {
		t.Errorf("expected getConnConfig type")
		return
	}
	if gc.GetConnConfig().User != changedUserName {
		t.Errorf("expected user changed to %s; got %s", changedUserName, gc.GetConnConfig().User)
	}
	if _, err = sshdbpgx.TunnelDriver.OpenConnector(dialer, dsn01); err == nil {
		t.Errorf("expected newconnector error; got <nil>")
	}
	sshdbpgx.SetConfigEdit(nil)
}

const testEnvName = "SSHDB_CONFIG_YAML_TEST_MYSQL"

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
	dbs, err := cfg.OpenDBs(sshdbpgx.TunnelDriver)
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
