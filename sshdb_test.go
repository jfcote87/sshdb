// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	"sync"
	"sync/atomic"
	"testing"

	"github.com/jfcote87/sshdb"
	"golang.org/x/crypto/ssh"
)

var unixSocket string

func TestTunnel(t *testing.T) {
	remoteDbAddr := []string{"localhost:9223", "localhost:9224"}
	if unixSocket > "" {
		remoteDbAddr = append(remoteDbAddr, unixSocket)
	}

	sshServerAddr := "127.0.0.1:9222"
	signer, serverSigner, err := getKeys()
	if err != nil {
		t.Errorf("unable to read keys - %v", err)
		return
	}

	ds := &directTCPServer{
		signer: serverSigner,
		key:    signer.PublicKey(),
		userID: "me",
		addr:   sshServerAddr,
		laddr:  remoteDbAddr,
		srvcfg: getPublicKeyServerCfg("me", signer.PublicKey()),
	}

	tunnel, err := sshdb.New(testOpener, ds.clientConfig(), ds.addr)
	if err != nil {
		t.Errorf("registration for %s failed %v", ds.addr, err)
		return
	}

	srvCloseFunc, err := ds.start()
	if err != nil {
		t.Errorf("%v", err)
		return
	}

	defer func() {
		srvCloseFunc()
	}()

	type connTests struct {
		dsn           string
		expectedCnt   int
		hasErr        bool
		hasConnectErr bool
	}

	var connectTests = []connTests{
		{dsn: remoteDbAddr[0]},
		{dsn: remoteDbAddr[1]},
		{dsn: "127.0.0.99:45632", hasConnectErr: true},
		{dsn: remoteDbAddr[1]},
		{dsn: "ERRlocal:3306", hasErr: true},
	}
	if len(remoteDbAddr) > 2 {
		connectTests = append(connectTests, connTests{
			dsn: remoteDbAddr[2], expectedCnt: 4,
		})
	}

	testLen := len(connectTests)
	var db = make([]*sql.DB, testLen, testLen)
	defer closeDBs(db)

	var wg sync.WaitGroup // used for concurrent ping calls
	var pingFuncs []func()
	var totalConnections int64

	var connectors = make([]driver.Connector, testLen)
	for i, connTest := range connectTests {
		connector, err := tunnel.OpenConnector(connTest.dsn)
		if err != nil {
			if !connTest.hasErr {
				t.Errorf("unable to open to connector[%d] db %v", i, err)
			}
			continue
		}
		if connTest.hasErr {
			t.Errorf("expected error on connector open[%d] db", i)
			continue
		}

		connectors[i] = connector
		db[i] = sql.OpenDB(connector)
		idx := i
		hasConnErr := connTest.hasConnectErr
		// have a good connector so run pings concurrently
		pingFuncs = append(pingFuncs, func() {
			defer wg.Done()
			if err := db[idx].PingContext(context.Background()); err != nil {
				if !hasConnErr {
					t.Errorf("unable to ping connTest[%d] db %v", idx, err)
				}
				return
			}
			atomic.AddInt64(&totalConnections, 1)
		})

	}
	wg.Add(len(pingFuncs))
	for _, f := range pingFuncs {
		go f()
	}
	wg.Wait()
	dialerCnt, err := sshdb.ConnectionCount(tunnel)
	if err != nil {
		t.Errorf("dialercount returned error %v", err)
		return
	}
	if dialerCnt != int(totalConnections) {
		t.Errorf("expected dialer count of %d; got %d", totalConnections, dialerCnt)
	}

	if err := tunnel.Close(); err != nil {
		t.Errorf("expect no errors with reset; got %v", err)
		return
	}
	cnt, err := sshdb.ConnectionCount(tunnel)
	if cnt != 0 {
		t.Errorf("expected no connections; dialer count is %d", cnt)
		return
	}

	dbx0 := sql.OpenDB(connectors[0])
	if err = dbx0.PingContext(context.Background()); err != nil {
		t.Errorf("unable to ping dbx0 after reset %v", err)
		return
	}
	if cnt, err := sshdb.ConnectionCount(tunnel); cnt != 1 || err != nil {
		t.Errorf("expected 1 connections; dialer count is %d - error %v", cnt, err)
	}
	dbx1 := sql.OpenDB(connectors[testLen-1])
	if err = dbx1.PingContext(context.Background()); err != nil {
		t.Errorf("unable to ping db01 after reset %v", err)
		return
	}
	if cnt, err := sshdb.ConnectionCount(tunnel); cnt != 2 || err != nil {
		t.Errorf("expected 2 connections; dialer count is %d - error %v", cnt, err)
	}
	dbx0.Close()
	if cnt, err := sshdb.ConnectionCount(tunnel); cnt != 1 || err != nil {
		t.Errorf("expected 1 connections; dialer count is %d - error %v", cnt, err)
	}
	dbx1.Close()
	if cnt, err := sshdb.ConnectionCount(tunnel); cnt != 0 || err != nil {
		t.Errorf("expected 0 connections; dialer count is %d - error %v", cnt, err)
	}

	testDriver := connectors[0].Driver()
	sql.Register("sshdb_tunnel", testDriver)

	connxErr, err := testDriver.Open("ERRlocal:3306")
	if err == nil {
		t.Errorf("expected tunnel.Open to fail with dsn = ERRlocal:3306")
		connxErr.Close()
	}
	// open 2 more connections
	defer tunnel.Close()
	connx2, err := testDriver.Open(remoteDbAddr[0])
	if err != nil {
		t.Errorf("expected connx2 success; got %v", err)
	}
	_ = connx2
	dbx2, err := sql.Open("sshdb_tunnel", remoteDbAddr[0])
	if err != nil {
		t.Errorf("dbx2 driver open %v", err)
		return
	}
	if err := dbx2.PingContext(context.Background()); err != nil {
		t.Errorf("dbx2 ping %v", err)
		return
	}
	// close underlying ClientConnection to simulatate net close
	if err := sshdb.CloseClient(tunnel); err != nil {
		t.Errorf("%v", err)
	}
	tunnel.Close() // ensure empty close
}

func closeDBs(dbs []*sql.DB) {
	for _, db := range dbs {
		if db != nil {
			db.Close()
		}
	}
}

type Resetter interface {
	Reset() error
}

func TestConnectionCount(t *testing.T) {
	expectedMsg := "expected a sshdb.tunnel"
	var drvctx driver.DriverContext
	_, err := sshdb.ConnectionCount(drvctx)
	if err == nil || err.Error() != expectedMsg {
		t.Errorf("expected %s, received %v", expectedMsg, err)
	}
}

func TestTunnel_Fail(t *testing.T) {
	remoteAddr, remoteDbAddr := "localhost:8222", []string{"localhost:8223"}
	_, serverSigner, err := getKeys()
	if err != nil {
		t.Errorf("unable to read keys - %v", err)
		return
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	password := "59d7b9-ee0625"
	testPass := "abcdefg"

	matchFunc := func(b []byte) bool {
		matched := string(b) == testPass
		if matched {
			cancelFunc()
		}
		return matched
	}

	ds := &directTCPServer{
		signer: serverSigner,
		key:    nil, //signer.PublicKey(),
		userID: "me",
		pwd:    password,
		addr:   remoteAddr,
		laddr:  remoteDbAddr,
		srvcfg: getPasswordServerCfg(matchFunc),
	}

	tunnel, err := sshdb.New(testOpener, ds.clientConfig(), ds.addr)
	if err != nil {
		t.Errorf("registration for %s failed %v", ds.addr, err)
		return
	}

	srvCloseFunc, err := ds.start()
	if err != nil {
		t.Errorf("%v", err)
		return
	}
	defer srvCloseFunc()

	connector00, err := tunnel.OpenConnector(remoteDbAddr[0])
	if err != nil {
		t.Errorf("unable to open connector00 %v", err)
		return
	}
	db00 := sql.OpenDB(connector00)
	defer db00.Close()
	if err := db00.Ping(); err == nil {
		t.Errorf("expecting connect error; got nil")
		return
	}
	testPass = password

	connector01, err := tunnel.OpenConnector(remoteDbAddr[0])
	if err != nil {
		t.Errorf("unable to open connector00 %v", err)
		return
	}

	db01 := sql.OpenDB(connector01)
	err = db01.PingContext(ctx)
	if err == nil || err.Error() != "context canceled" {
		t.Errorf("expected context canceled; got %v", err)
	}
	db01.Close()
}

func TestDeadlines(t *testing.T) {
	remoteAddr, remoteDbAddr := "localhost:8222", []string{"localhost:8223"}
	_, serverSigner, err := getKeys()
	if err != nil {
		t.Errorf("unable to read keys - %v", err)
		return
	}

	ctx := context.Background()
	matchFunc := func(b []byte) bool {
		return true
	}

	ds := &directTCPServer{
		signer: serverSigner,
		key:    nil, //signer.PublicKey(),
		userID: "me",
		pwd:    "abcd",
		addr:   remoteAddr,
		laddr:  remoteDbAddr,
		srvcfg: getPasswordServerCfg(matchFunc),
	}

	tunnel, err := sshdb.New(testOpener, ds.clientConfig(), ds.addr)
	if err != nil {
		t.Errorf("registration for %s failed %v", ds.addr, err)
		return
	}

	srvCloseFunc, err := ds.start()
	if err != nil {
		t.Errorf("%v", err)
		return
	}
	defer srvCloseFunc()

	connector00, err := tunnel.OpenConnector(remoteDbAddr[0])
	if err != nil {
		t.Errorf("unable to open connector00 %v", err)
		return
	}
	db00 := sql.OpenDB(connector00)
	defer db00.Close()
	var deadlineType = []string{"deadline", "readdeadline", "writedeadline"}
	ctx = context.WithValue(ctx, "deadlines", "ignore")
	err = db00.PingContext(ctx)
	elx, ok := err.(errlist)
	if !ok {
		t.Errorf("expected errlist; got %#v", err)
		return
	}
	for i, err := range elx {
		if err != nil {
			t.Errorf("expected %s to return nil; got %v", deadlineType[i], err)
		}
	}
	ctx = context.WithValue(ctx, "deadlines", "default")
	err = db00.PingContext(ctx)
	elx, ok = err.(errlist)
	if !ok {
		t.Errorf("expected errlist; got %#v", err)
		return
	}
	for i, err := range elx {
		if err == nil || err.Error() != "ssh: tcpChan: deadline not supported" {
			t.Errorf("expected %s to return ssh: tcpChan: deadline not supported; got %v", deadlineType[i], err)
		}
	}

}

func TestNewTunnel(t *testing.T) {
	type args struct {
		addr string
		cfg  *ssh.ClientConfig
		cc   sshdb.ConnectorOpener
	}
	var cfg ssh.ClientConfig
	tests := []struct {
		name      string
		args      args
		errString string
	}{

		{name: "err00", args: args{cc: testOpener, addr: "localhost:22"}, errString: "clientConfig may not be nil"},
		{name: "err01", args: args{cc: testOpener, cfg: &cfg}, errString: "remoteAddr may not be empty"},
		{name: "err02", args: args{addr: "localhost:22", cfg: &cfg}, errString: "opener may not be nil"},
		{name: "ok", args: args{cc: testOpener, addr: "work:22", cfg: &cfg}, errString: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tunnel, err := sshdb.New(tt.args.cc, tt.args.cfg, tt.args.addr)
			if err == nil && tt.errString == "" {

				if getName(tunnel) != testOpener.Name() {
					t.Errorf("expected dbname = %s; got %s", testOpener.Name(), getName(tunnel))
				}
				return
			}
			if err == nil || err.Error() != tt.errString {
				t.Errorf("expected err of %q; got %v", tt.errString, err)
			}
		})
	}
}

type DBNamer interface {
	DBName() string
}

func getName(tunnel sshdb.Tunnel) string {
	if nm, ok := tunnel.(DBNamer); ok {
		return nm.DBName()
	}
	return ""
}

func getPublicKeyServerCfg(userID string, key ssh.PublicKey) *ssh.ServerConfig {
	publicKeyBytes := key.Marshal()
	publicKeyType := key.Type()
	return &ssh.ServerConfig{

		PublicKeyCallback: func(meta ssh.ConnMetadata, pk ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() != userID {
				return nil, fmt.Errorf("%s is an invalid user", userID)
			}
			if publicKeyType != key.Type() {
				return nil, fmt.Errorf("%d expected cert type %s, got %s", len(publicKeyType), publicKeyType, key.Type())
			}
			if bytes.Compare(publicKeyBytes, key.Marshal()) != 0 {
				return nil, fmt.Errorf("invalid key")
			}
			return &ssh.Permissions{
				Extensions: map[string]string{
					"user": meta.User(),
				},
			}, nil
		},
	}
}

func getPasswordServerCfg(matchFunc func([]byte) bool) *ssh.ServerConfig {
	return &ssh.ServerConfig{
		//NoClientAuth: false,
		PasswordCallback: func(meta ssh.ConnMetadata, pwd []byte) (*ssh.Permissions, error) {
			if matchFunc(pwd) {
				return &ssh.Permissions{
					Extensions: map[string]string{
						"user": meta.User(),
					},
				}, nil
			}
			return nil, fmt.Errorf("invalid password")
		},
	}
}
