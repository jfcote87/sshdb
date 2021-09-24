// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mysql_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/jfcote87/sshdb"

	"github.com/jfcote87/sshdb/internal"
	"github.com/jfcote87/sshdb/mysql"
)

func TestOpener(t *testing.T) {

	if mysql.Opener.Name() != "mysql" {
		t.Errorf("expected ConnectorOpener.Name() = \"mysql\"; got %s", mysql.Opener.Name())
	}

	ctx, cancelfunc := context.WithCancel(context.Background())
	defer cancelfunc()
	cnt := 1
	var dialer sshdb.Dialer = sshdb.DialerFunc(func(ctxx context.Context, net, dsn string) (net.Conn, error) {
		cancelfunc()
		cnt++
		return nil, errors.New("no connect")
	})

	connectorFail, err := mysql.Opener.NewConnector(dialer, "sa:passwordzzz(localhost:3306)schema")
	if err == nil {
		t.Errorf("connectorfail expected dsn error ; got %v", err)
		return
	}
	_ = connectorFail

	connector, err := mysql.Opener.NewConnector(dialer, "sa:password@tcp(localhost:3306)/schema")
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

func TestOpener_live(t *testing.T) {
	db, err := internal.DBFromEnvSettings(mysql.Opener)
	if err != nil {
		if err == internal.ErrNoEnvVariable {
			t.Skip("test connection skipped, SSHDB_CLIENT_CONNECTION_MYSQL not found")
			return
		}
		t.Errorf("%v", err)
		return
	}
	if err = db.Ping(); err != nil {
		t.Errorf("ping failure %v", err)
	}
}
