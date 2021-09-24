// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/internal"
)

func TestDBFromEnvSettings(t *testing.T) {

	tests := []struct {
		env       string
		want      *sql.DB
		wantErr   bool
		errStr    string
		openerIdx int
	}{
		{env: "", wantErr: true, errStr: internal.ErrNoEnvVariable.Error()},
		{env: "work:22", wantErr: true, errStr: "user not specified"},
		{env: "work:22,user,pwd", wantErr: true, errStr: "empty dsn"},
		{env: "work:22,user,pwd,,,,dsn string"},
		{env: "work:22,user,pwd,testfiles/no_key,,,dsn string", wantErr: true, errStr: "unable to read file"},
		{env: "work:22,user,pwd,testfiles/test_key,,,dsn string", wantErr: false},
		{env: "work:22,user,pwd,testfiles/test_pwd_key,my_favorite_password,,dsn string", wantErr: false, errStr: "unable to read file"},
		{env: "work:22,user,pwd,testfiles/test_pwd_key,my_next_favorite_password,,dsn string", wantErr: true, errStr: "x509: decryption password incorrect"},
		{env: "work:22,user,pwd,,,testfiles/server_key.pub,dsn string", wantErr: false, errStr: "x509: decryption password incorrect"},
		{env: "work:22,user,pwd,,,testfiles/no_key.pub,dsn string", wantErr: true, errStr: "unable to read public key"},
		{env: "work:22,user,pwd,,,,bad dsn string", wantErr: true, errStr: "bad dsn string"},
		{env: ",user,pwd,,,,openconnector fail", wantErr: true, errStr: "unable to open internal tunne"},
		{env: ",user,pwd,,,,openconnector fail", wantErr: true, errStr: "opener may not be nil", openerIdx: 1},
	}
	var openers = []sshdb.ConnectorOpener{Opener, nil}
	for i, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			os.Setenv("SSHDB_CLIENT_CONNECTION_INTERNAL", tt.env)

			_, err := internal.DBFromEnvSettings(openers[tt.openerIdx])
			if err != nil || tt.wantErr {
				if !tt.wantErr {
					t.Errorf("%d DBFromEnvSettings() wanted success, got %v", i, err)
					return
				}
				teststr := fmt.Sprintf("%v", err)
				if !strings.HasPrefix(teststr, tt.errStr) {
					t.Errorf("%d DBFromEnvSettings() wanted %v, got %v", i, tt.errStr, err)
				}
				return
			}

		})
	}
}

type Driver struct {
	failOpenConnector bool
}

func (d *Driver) OpenConnector(dsn string) (*Connector, error) {
	if dsn == "openconnector fail" {
		return nil, errors.New("openconnector fail")
	}
	return &Connector{}, nil
}

func (d *Driver) Open(nm string) (driver.Conn, error) {
	return nil, nil
}

type Connector struct {
	dx *Driver
}

func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	return nil, errors.New("mock connector")
}

func (c *Connector) Driver() driver.Driver {
	return c.dx
}

var Opener sshdb.ConnectorOpener = opener("internal")

func (o opener) NewConnector(dialer sshdb.Dialer, dsn string) (driver.Connector, error) {
	if dsn == "" {
		return nil, errors.New("empty dsn")
	}
	if dsn == "bad dsn string" {
		return nil, errors.New("bad dsn string")
	}
	return &Connector{}, nil
}

type opener string

func (o opener) Name() string {
	return string(o)
}
