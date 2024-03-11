## sshdb

[![Go Reference](https://pkg.go.dev/badge/github.com/jfcote87/sshdb.svg)](https://pkg.go.dev/github.com/jfcote87/sshdb) [![Build Status](https://app.travis-ci.com/jfcote87/sshdb.svg?branch=main)](https://app.travis-ci.com/jfcote87/sshdb) [![codecov](https://codecov.io/gh/jfcote87/sshdb/branch/main/graph/badge.svg?token=6WUH2GPZ0T)](https://codecov.io/gh/jfcote87/sshdb) [![Go Report Card](https://goreportcard.com/badge/github.com/jfcote87/sshdb)](https://goreportcard.com/report/github.com/jfcote87/sshdb)


A pure go library that allows connections to remote sql databases via ssh tunnels. The package
works with db packages by creating an ssh connection and a dial function for connecting to the 
tunnel. 

## install

go get -v github.com/jfcote87/sshdb

## making connections

To setup connections, create a Tunnel object with the sshdb.New method using an ssh.ClientConfig (see documentation on the "golang.org/x/crypto/ssh" package) and the remote address of the ssh server.  The returned Tunnel is safe for concurrent use by multiple goroutines and maintains its own pool of db connections. Thus, the New function should be called just once for each remote server. It is rarely necessary to close a Tunnel.

Create a dsn string using addresses based upon the remote server.  The connections will be created on the remote ssh server to the database server.  Select the appropriate driver for your database.  If the database is not in included drivers, review the sshdb.Driver interface{} and the existing driver code.  Essentially the passed dialer in the OpenConnector function should replace the default dialer for the db.  Use the db.Connector to create a sql.DB object.

Example for creating connections may be found in the code below and the example_test.go file.

```
imports (
	"golang.org/x/crypto/ssh"
	"github.com/jfcote87/sshdb"
)

func main() {
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
}
```

## testing

    $ go test ./...






