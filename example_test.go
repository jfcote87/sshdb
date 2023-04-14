//Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb_test

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/mssql"
	"github.com/jfcote87/sshdb/mysql"
	"golang.org/x/crypto/ssh"
)

// ExampleNew demonstrates the package's simplest usage,
// accessing a single mysql server on a remote host where port
// 3306 is blocked but the remote host is accessible via ssh.
func ExampleNew() {
	var (
		// values used in connecting remote host
		remoteAddr = "remote.example.com:22"

		ctx, cancelFunc = context.WithCancel(context.Background())
	)
	defer cancelFunc()

	signer, serverSigner, _ := getKeys()
	exampleCfg := &ssh.ClientConfig{
		User: "me",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.FixedHostKey(serverSigner.PublicKey()),
	}
	// New creates a "tunnel" for database connections.
	tunnel, err := sshdb.New(exampleCfg, remoteAddr)
	if err != nil {
		log.Fatalf("new tunnel create failed: %v", err)
	}

	configs := []struct {
		nettype      string
		dbServerAddr string
	}{
		{"tcp", "localhost:3306"},      // local database on remote server tcp connection
		{"unix", "/tmp/mysql.sock"},    // local database on remote server via unix socket
		{"tcp", "db.example.com:3306"}, // connect to db.example.com db from remote server skirt around a firewall
	}
	for _, cfg := range configs {

		// dbServerAddr is a valid address for the db server beginning from the remote ssh server.
		dsn := fmt.Sprintf("username:password@%s(%s)/schemaname?parseTime=true", cfg.nettype, cfg.dbServerAddr)

		// open connector and then new DB
		connector, err := tunnel.OpenConnector(mysql.TunnelDriver, dsn)
		if err != nil {
			log.Printf("open connector failed %s - %v", dsn, err)
			continue
		}
		db := sql.OpenDB(connector)
		defer db.Close()

		// ping tests connectivity
		if err := db.PingContext(ctx); err != nil {
			log.Printf("%v ping failed: %v", cfg.dbServerAddr, err)
		}
	}
}

// ExampleNew_multiplehosts demonstrates how to connect
// multiple remote database servers via a single ssh connection
func ExampleNew_multipledbservers() {
	var (
		// values used in connecting remote host
		remoteAddr = "remote.example.com:22"

		// values used in dsn string
		dbServerAddr    = []string{"localsrv.example.com", "cloudsvr.example.com"}
		ctx, cancelFunc = context.WithCancel(context.Background())
	)
	defer cancelFunc()
	exampleCfg := &ssh.ClientConfig{
		User:            "jfcote87",
		Auth:            []ssh.AuthMethod{ssh.Password("my second favorite password")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	tunnelCtx, err := sshdb.New(exampleCfg, remoteAddr)
	if err != nil {
		log.Fatalf("newDriverContext00 failed: %v", err)
	}

	dsn00 := fmt.Sprintf("uid=me;password=xpwd;server=%s;database=crm", dbServerAddr[0])
	dsn01 := fmt.Sprintf("uid=me;password=ypwd;server=%s;database=web", dbServerAddr[1])

	connector00, err := tunnelCtx.OpenConnector(mssql.TunnelDriver, dsn00)
	if err != nil {
		log.Fatalf("open connector failed %s - %v", dsn00, err)
	}
	connector01, err := tunnelCtx.OpenConnector(mssql.TunnelDriver, dsn01)
	if err != nil {
		log.Fatalf("open connector failed %s - %v", dsn01, err)
	}

	db00, db01 := sql.OpenDB(connector00), sql.OpenDB(connector01)

	defer db00.Close()
	defer db01.Close()
	// ping tests connectivity
	if err := db00.PingContext(ctx); err != nil {
		log.Printf("%s ping failed: %v", "db0.example.com", err)
	}
	if err := db01.PingContext(ctx); err != nil {
		log.Printf("%s ping failed: %v", "db1.example.com", err)
	}
}

func getKeys() (ssh.Signer, ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKeyWithPassphrase([]byte(clientPrivateKey),
		[]byte("sshdb_example"))
	if err != nil {
		return nil, nil, fmt.Errorf("private key parse error: %v", err)
	}
	serverSigner, err := ssh.ParsePrivateKey([]byte(serverPrivateKey))
	if err != nil {
		return nil, nil, fmt.Errorf("server private key parse error: %v", err)
	}
	return signer, serverSigner, nil
}

// clientPrivateKey is used to authenticate with the remote ssh server.
// This key was generated using the following command
// ssh-keygen -f ~/sshdb_client_key -t ecdsa -b 521
const clientPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAACmFlczI1Ni1jdHIAAAAGYmNyeXB0AAAAGAAAABCrg/C49e
zn3txdMKskd0JiAAAAEAAAAAEAAACsAAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlz
dHA1MjEAAACFBAHy1nPGt0hS9cS7ENbslUy28NC5ubYw/pdlm4w/ugkudkydOSbn+q6Hsk
VM8Q8RJP71oTOV2BWCYN5wMrk6LYTQ+QDpVDA0MHjs1ZHfhwciVZWG+RaJTZcLEhAHfUjL
v8JPPAc4q3ygNNHUJUSWY/37rJzJ0GNJU2aiEuO6dKzXb8Z1dwAAARDBuo7xtZHjwwMbS7
EExM4NzO45Hq21lPPhWcRhht90bpsG8pVG69Vb4PIo9khQDm4WfPLI/a0Vujrvj4oSckNP
ay7DN6sTtVWbfInJbt1Rm1FuECQMIakEapQmPrjQyMWHREfgM0GaRgHIAy/9KXSD1rq7co
MmWA8Jmmg7xa8wL/c/fgtB3q0vDBU5jdZHu5b/uQgdDoiZm7gwLxny0AVVWFTetpspTMbh
cmihTM9+44fHkIzhCpMzDVb8uR+FnSmjyj6GGghJtagwNm151Y3JXjNGPlRUi7VBnbE7LC
wXxGJwJo8diI8o0ew25P+n3K26eVHKfSvwljLjdBS5GeFyJE35ul4QsO2w+t0cAjj/SQ==
-----END OPENSSH PRIVATE KEY-----
`

// serverPrivateKey is used to authenticate ssh clients.
// This key was generated using the following command
// ssh-keygen -f ~/sshdb_server_key -t ecdsa -b 521
const serverPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAArAAAABNlY2RzYS
1zaGEyLW5pc3RwNTIxAAAACG5pc3RwNTIxAAAAhQQAaHYSCQ8ultHfdGu2LeDfR4uM8M5r
DwNziz1bwy2J57/1fZm4j4BBBNnqEXfgQwscnn2bJqoAVS8BtSKz4uA9CrEAMbTuu6FK7m
UyEKllyZ6RfdwUjBClYRsb8qvcrC2KJDNYePASZs8ufgCASEWZ2bNoZSJHooMFwOXL5q17
vDOJHqUAAAEQaQKgqGkCoKgAAAATZWNkc2Etc2hhMi1uaXN0cDUyMQAAAAhuaXN0cDUyMQ
AAAIUEAGh2EgkPLpbR33Rrti3g30eLjPDOaw8Dc4s9W8Mtiee/9X2ZuI+AQQTZ6hF34EML
HJ59myaqAFUvAbUis+LgPQqxADG07ruhSu5lMhCpZcmekX3cFIwQpWEbG/Kr3KwtiiQzWH
jwEmbPLn4AgEhFmdmzaGUiR6KDBcDly+ate7wziR6lAAAAQgDVggCI6pefB2znhtdT187I
iWZU7LTARxroTZqJzJRT3nvmu1IBV3FY0v6VXbpYoREpRfDnp8aLt2S3cPw2x8yMOwAAAA
xyb290QEpGQy1TTUcBAgMEBQY=
-----END OPENSSH PRIVATE KEY-----
`
