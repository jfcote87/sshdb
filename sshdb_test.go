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
	"strings"
	"sync"
	"testing"

	"github.com/jfcote87/sshdb"
	"golang.org/x/crypto/ssh"
)

var unixSocket string

type testTunnelConfig struct {
	tun          *sshdb.Tunnel
	cancel       func()
	remoteDbAddr []string
	connectors   []driver.Connector
	connectTests []connTests
	hasErr       bool
}

type connTests struct {
	dsn           string
	expectedCnt   int
	hasErr        bool
	hasConnectErr bool
}

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
	srvCloseFunc, err := ds.start()
	if err != nil {
		t.Errorf("directTCPServer start %v", err)
		return
	}
	defer srvCloseFunc()
	tunnel, err := sshdb.New(ds.clientConfig(), ds.addr) // testDriver, ds.clientConfig(), ds.addr)
	if err != nil {
		t.Errorf("registration for %s failed %v", ds.addr, err)
		return
	}
	defer func() {
		tunnel.Close()
	}()
	cfg := &testTunnelConfig{
		tun:          tunnel,
		cancel:       srvCloseFunc,
		remoteDbAddr: remoteDbAddr,
	}

	cfg.connectTests = []connTests{
		{dsn: remoteDbAddr[0]},
		{dsn: remoteDbAddr[1]},
		{dsn: "127.0.0.99:45632", hasConnectErr: true},
		{dsn: remoteDbAddr[1]},
		{dsn: "ERRlocal:3306", hasErr: true},
	}
	if len(remoteDbAddr) > 2 {
		cfg.connectTests = append(cfg.connectTests, connTests{
			dsn: remoteDbAddr[2], expectedCnt: 4,
		})
	}

	if cfg.testConnectPing(t); cfg.hasErr {
		return
	}
	if cfg.testOpenDB(t); cfg.hasErr {
		return
	}
	if cfg.testOpen(t); cfg.hasErr {
		return
	}
}

func (cfg *testTunnelConfig) testConnectPing(t *testing.T) {
	cfg.hasErr = true
	tunnel := cfg.tun
	connectTests := cfg.connectTests
	testLen := len(connectTests)
	ptp := &pingTestParams{
		cfg: cfg,
		dbs: make([]*sql.DB, len(connectTests)),
	}
	defer closeDBs(ptp.dbs)

	cfg.connectors = make([]driver.Connector, testLen)

	ptp.funcs(t)
	ptp.wg.Add(len(ptp.pingFuncs))
	for _, f := range ptp.pingFuncs {
		go f()
	}
	ptp.wg.Wait()
	dialerCnt, err := sshdb.ConnectionCount(tunnel)
	if err != nil {
		t.Errorf("dialercount returned error %v", err)
		return
	}
	if dialerCnt != ptp.totalConnections {
		t.Errorf("expected dialer count of %d; got %d", ptp.totalConnections, dialerCnt)
	}
	if err := tunnel.Close(); err != nil {
		t.Errorf("expect no errors with reset; got %v", err)
		return
	}
	cnt, err := sshdb.ConnectionCount(tunnel)
	if cnt != 0 || err != nil {
		t.Errorf("expected no connections; dialer count is %d %v", cnt, err)
		return
	}
	cfg.hasErr = false
}

type pingTestParams struct {
	cfg              *testTunnelConfig
	dbs              []*sql.DB
	totalConnections int
	pingFuncs        []func()
	wg               sync.WaitGroup
}

func (ptp *pingTestParams) funcs(t *testing.T) {
	var mTotalConnections sync.Mutex
	for i, connTest := range ptp.cfg.connectTests {
		connector, err := ptp.cfg.tun.OpenConnector(testDriver, connTest.dsn)
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
		idx := i
		ptp.cfg.connectors[idx] = connector
		ptp.dbs[idx] = sql.OpenDB(connector)

		hasConnErr := connTest.hasConnectErr
		// have a good connector so run pings concurrently
		ptp.pingFuncs = append(ptp.pingFuncs, func() {
			defer ptp.wg.Done()
			if connTest.hasConnectErr {
				t.Errorf("Has connecterror")
			}
			if err := ptp.dbs[idx].PingContext(context.Background()); err != nil {
				if !hasConnErr {
					t.Errorf("unable to ping connTest[%d] db %v", idx, err)
				}
				return
			}
			mTotalConnections.Lock()
			ptp.totalConnections++
			mTotalConnections.Unlock()
		})
	}
}

func (cfg *testTunnelConfig) testOpenDB(t *testing.T) {
	cfg.hasErr = true
	dbx0 := sql.OpenDB(cfg.connectors[0])
	defer dbx0.Close()
	if err := dbx0.PingContext(context.Background()); err != nil {
		t.Errorf("unable to ping dbx0 after reset %v", err)
		return
	}
	if err := connectionCountTest(cfg.tun, 1); err != nil {
		t.Errorf("%v", err)
	}
	dbx1 := sql.OpenDB(cfg.connectors[len(cfg.connectTests)-1])
	defer dbx1.Close()
	if err := dbx1.PingContext(context.Background()); err != nil {
		t.Errorf("unable to ping db01 after reset %v", err)
		return
	}
	if err := connectionCountTest(cfg.tun, 2); err != nil {
		t.Errorf("%v", err)
	}
	dbx0.Close()
	if err := connectionCountTest(cfg.tun, 1); err != nil {
		t.Errorf("%v", err)
	}
	dbx1.Close()
	if err := connectionCountTest(cfg.tun, 0); err != nil {
		t.Errorf("%v", err)
		return
	}
	cfg.hasErr = false
}

func connectionCountTest(tun *sshdb.Tunnel, expectedCnt int) error {
	cnt, err := sshdb.ConnectionCount(tun)
	if err != nil {
		return fmt.Errorf("expected %d connection; got error %v", expectedCnt, err)
	}
	if cnt != expectedCnt {
		return fmt.Errorf("expected %d connections; dialer count is %d", expectedCnt, cnt)
	}
	return nil
}

func (cfg *testTunnelConfig) testOpen(t *testing.T) {
	cfg.hasErr = true
	// test Open legacy methods
	testDriver := cfg.connectors[0].Driver()
	sql.Register("sshdb_tunnel", testDriver)

	connxErr, err := testDriver.Open("ERRlocal:3306")
	if err == nil {
		t.Errorf("expected tunnel.Open to fail with dsn = ERRlocal:3306")
		connxErr.Close()
	}
	connx2, err := testDriver.Open(cfg.remoteDbAddr[0])
	if err != nil {
		t.Errorf("expected connx2 success; got %v", err)
	}
	_ = connx2
	dbx2, err := sql.Open("sshdb_tunnel", cfg.remoteDbAddr[0])
	if err != nil {
		t.Errorf("dbx2 driver open %v", err)
		return
	}
	if err := dbx2.PingContext(context.Background()); err != nil {
		t.Errorf("dbx2 ping %v", err)
		return
	}
	// close underlying ClientConnection to simulatate net close
	if err := sshdb.CloseClient(cfg.tun); err != nil {
		t.Errorf("%v", err)
		return
	}
	cfg.hasErr = false
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

	tunnel, err := sshdb.New(ds.clientConfig(), ds.addr)
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

	connector00, err := tunnel.OpenConnector(testDriver, remoteDbAddr[0])
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

	connector01, err := tunnel.OpenConnector(testDriver, remoteDbAddr[0])
	if err != nil {
		t.Errorf("unable to open connector00 %v", err)
		return
	}

	db01 := sql.OpenDB(connector01)
	err = db01.PingContext(ctx)
	if err != context.Canceled {
		t.Errorf("expected context canceled; got %v", err)
	}
	db01.Close()
}

type deadlineKeyType struct{}

var deadlineKey deadlineKeyType

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

	tunnel, err := sshdb.New(ds.clientConfig(), ds.addr)
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

	connector00, err := tunnel.OpenConnector(testDriver, remoteDbAddr[0])
	if err != nil {
		t.Errorf("unable to open connector00 %v", err)
		return
	}
	db00 := sql.OpenDB(connector00)
	defer db00.Close()
	ctx = context.WithValue(ctx, deadlineKey, func() {
		tunnel.IgnoreSetDeadlineRequest(true)
	})
	for _, errs := range checkDeadlines(ctx, db00, true) {
		if errs != nil {
			t.Errorf("%v", errs)
		}
	}
	ctx = context.WithValue(ctx, deadlineKey, func() {
		tunnel.IgnoreSetDeadlineRequest(false)
	})
	for _, errs := range checkDeadlines(ctx, db00, false) {
		if errs != nil {
			t.Errorf("%v", errs)
		}
	}
}

func checkDeadlines(ctx context.Context, db00 *sql.DB, ignore bool) errlist {
	var deadlineType = []string{"deadline", "readdeadline", "writedeadline"}

	err := db00.PingContext(ctx)
	var retlist = make(errlist, len(deadlineType))

	elx, ok := err.(errlist)
	if !ok {
		retlist[0] = fmt.Errorf("expected errlist; got %#v", err)
		return retlist
	}
	if len(elx) != 3 {
		retlist[0] = fmt.Errorf("expected errlist with len 3; got %d", len(elx))
	}
	for i, err := range elx {
		if ignore {
			if err != nil {
				retlist[i] = fmt.Errorf("expected %s to return nil; got %v", deadlineType[i], err)
			}
			continue
		}
		if err == nil || err.Error() != "ssh: tcpChan: deadline not supported" {
			retlist[i] = fmt.Errorf("expected %s to return ssh: tcpChan: deadline not supported; got %v", deadlineType[i], err)
		}
	}
	return retlist
}

func TestNewTunnel(t *testing.T) {
	type args struct {
		addr string
		cfg  *ssh.ClientConfig
		cc   sshdb.Driver
	}
	var cfg ssh.ClientConfig
	tests := []struct {
		name      string
		args      args
		errString string
	}{

		{name: "err00", args: args{cc: testDriver, addr: "localhost:22"}, errString: "clientConfig may not be nil"},
		{name: "err01", args: args{cc: testDriver, cfg: &cfg}, errString: "remoteAddr may not be empty"},
		{name: "err02", args: args{cc: testDriver, addr: "ssh.example.com", cfg: &cfg}, errString: "invalid address"},
		{name: "ok", args: args{cc: testDriver, addr: "work:22", cfg: &cfg}, errString: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tunnel, err := sshdb.New(tt.args.cfg, tt.args.addr)
			if tt.errString == "" && err == nil {
				tunnel.Close()
				return
			}
			if err == nil || !strings.HasPrefix(err.Error(), tt.errString) {
				t.Errorf("expected err of %q; got %v", tt.errString, err)
			}
		})
	}
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
			if !bytes.Equal(publicKeyBytes, key.Marshal()) {
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
