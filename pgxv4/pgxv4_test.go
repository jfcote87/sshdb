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
	"net"
	"testing"

	"github.com/jackc/pgx/v4"
	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/internal"
	"github.com/jfcote87/sshdb/pgxv4"
)

func TestOpener(t *testing.T) {
	if pgxv4.Opener.Name() != "postgres_pgxv4" {
		t.Errorf("expected ConnectorCreator.Name() = \"postgres_pgxv4\"; got %s", pgxv4.Opener.Name())
	}
	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()

	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		cancelfunc()
		return nil, errors.New("no connect")
	})
	connectorFail, err := pgxv4.Opener.NewConnector(dialer, "applic2.3.4 dbname=mydb")
	if err != nil {
		t.Errorf("connectorfail expected \"unexpected character error\"; got %v", err)
		return
	}
	_ = connectorFail
	connector, err := pgxv4.Opener.NewConnector(dialer, "postgres://jack:secret@10.52.32.93:5432/mydb?sslmode=verify-ca")
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
	_, err := pgxv4.Opener.NewConnector(dialer, dsn00)
	if err != nil {
		t.Errorf("dsn00 expected successful open; got %v", err)
		return
	}

	if _, err = pgxv4.Opener.NewConnector(dialer, dsn01); err == nil {
		t.Errorf("dsn01 expected newconnector error; got <nil>")

	}
	if _, err := pgxv4.Opener.NewConnector(dialer, dsn02); err == nil {
		t.Errorf("dsn02 expected newconnector error; got <nil>")
	}
	pgxv4.SetConfigEdit(nil)
}

func TestOpener_live(t *testing.T) {
	db, err := internal.DBFromEnvSettings(pgxv4.Opener)
	if err != nil {
		if err == internal.ErrNoEnvVariable {
			t.Skip("test connection skipped, SSHDB_CLIENT_CONNECTION_POSTGRES_PGXV4 not found")
			return
		}
		t.Errorf("%v", err)
		return
	}
	if err = db.Ping(); err != nil {
		t.Errorf("ping failure %v", err)
	}
}
