## sshdb

[![Go Reference](https://pkg.go.dev/badge/github.com/jfcote87/sshdb.svg)](https://pkg.go.dev/github.com/jfcote87/sshdb) [![Build Status](https://app.travis-ci.com/jfcote87/sshdb.svg?branch=main)](https://app.travis-ci.com/jfcote87/sshdb) [![codecov](https://codecov.io/gh/jfcote87/sshdb/branch/main/graph/badge.svg?token=6WUH2GPZ0T)](https://codecov.io/gh/jfcote87/sshdb) [![Go Report Card](https://goreportcard.com/badge/github.com/jfcote87/sshdb)](https://goreportcard.com/report/github.com/jfcote87/sshdb)


A pure go library that provides an ssh wrapper (using golang.org/x/crypt) for connecting a database client to a remote database. 

## install

go get -v github.com/jfcote87

## making connections

The workflow is straight forward.  Create a Tunnel and then
create sql.DB connection using the tunnel.  The first method for 
accomplishing this is by creating an ssh client connection and
create a tunnel using the connection.  Finally open a *sql.DB
using a dataSourceName string.  The dsn should defing a connection
from the ssh remote server and the desired database.

	exampleCfg := &ssh.ClientConfig{
		User:            "jfcote87",
		Auth:            []ssh.AuthMethod{ssh.Password("my second favorite password")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	// New creates a "tunnel" for database connections. A Tunnel can support
	// multiple database connections.
	tunnel, err := sshdb.New(exampleCfg, remoteAddr)
	if err != nil {
		log.Fatalf("new tunnel create failed: %v", err)
	}
    // serverAddr is a valid hostname for the db server from the remote ssh server (often localhost).
	dsn := "username:dbpassword@tcp(serverAddress:3306)/schemaName?parseTime=true"

	// open connector and then new DB
	connector, err := tunnel.OpenConnector(mysql.TunnelDriver, dsn)
	if err != nil {
		return fmt.Errorf("open connector failed %s - %v", dsn, err)
	}
	db := sql.OpenDB(connector)

You can also use a TunnelConfig defines the ssh tunnel as well as 
one or more database connections using database/driver names and
dataSourceName definitions.  Example TunnelConfig definitions may
be found in the example_config_test.db file and in the 
testfiles/config directory.

	var config *sshdb.TunnelConfig
	if err := yaml.Unmarshal([]byte(cfg_yaml), &config); err != nil {
		log.Fatalf("yaml decode failed: %v", err)
	}
	dbs, err := config.DatabaseMap()
	if err != nil {
		log.Fatalf("opendbs fail: %v", err)
	}
	dbs["remoteDB"].Ping()



	

## testing

    $ go test ./...






