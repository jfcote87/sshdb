// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"sync"

	"golang.org/x/crypto/ssh"
)

var driverMap = make(map[string]Driver)
var mDriverMap sync.Mutex

// RegisterDriver associates a database's Driver interface
// with the key.  This allows a TunnelConfig to handle
// multiple database types per host connection.
func RegisterDriver(key string, driver Driver) {
	mDriverMap.Lock()
	driverMap[key] = driver
	mDriverMap.Unlock()
}

// Datasource defines a database connection using the
// Driver name and a connection string for use by the
// underlying sql driver. The DriverName must be registered
// or the TunnelConfig.OpenDB will return an error.
type Datasource struct {
	DriverName       string `yaml:"driver_name" json:"driver_name,omitempty"`
	ConnectionString string `yaml:"dsn" json:"dsn,omitempty"`
	// tests use this parameter
	Queries []string `yaml:"queries,omitempty" json:"queries,omitempty"`
}

// Driver returns the Driver associated with the
// ConnDefinition.DriverName.  Will return error if the
// name was not associated using the RegisterDriver func.
func (cd Datasource) Driver() (Driver, error) {

	drv := driverMap[string(cd.DriverName)]
	if drv == nil {
		return nil, fmt.Errorf("no driver found for [%s]", cd.DriverName)
	}
	return drv, nil
}

func parseKey(b []byte, pwd string) (ssh.Signer, error) {
	if pwd > "" {
		return ssh.ParsePrivateKeyWithPassphrase(b, []byte(pwd))
	}
	return ssh.ParsePrivateKey(b)
}

func parsePubKey(b []byte) (ssh.PublicKey, error) {
	k, _, _, _, err := ssh.ParseAuthorizedKey(b)
	return k, err
}

// TunnelConfig describes an ssh connection to a remote host and the databases
// accessed via the connection.  See the example_config_test.go file for examples.
type TunnelConfig struct {
	// address of remote server must be in the form "host:port", "host%zone:port",
	// "[host]:port" or "[host%zone]:port".  See func net.Dial for a description of
	// the hostport parameter.
	HostPort string `yaml:"hostport,omitempty" json:"hostport,omitempty"`
	// login name for the remote ssh connection
	UserID string `yaml:"user_id,omitempty" json:"user_id,omitempty"`
	// password to use with UserID.  May be blank if using keys.
	Pwd string `yaml:"pwd,omitempty" json:"pwd,omitempty"`
	// file containing a PEM version of the private key used for authenticating the ssh session
	ClientKeyFile string `yaml:"client_key_file,omitempty" json:"client_key_file,omitempty"`
	// string containing PEM of private key.  May not use ClientKeyFile and ClientKey simultaneously
	ClientKey string `yaml:"client_key,omitempty" json:"client_key,omitempty"`
	// if private key is phrase protected, set this to password phrase.  Otherwise leave blank
	ClientKeyPwd string `yaml:"client_key_pwd,omitempty" json:"client_key_pwd,omitempty"`
	// file containing public key for validating remote host.  ServerPublicKeyFile and ServerPublicKey may
	// not be used simultaneously.
	ServerPublicKeyFile string `yaml:"server_public_key_file,omitempty" json:"server_public_key_file,omitempty"`
	// string containing public key definition.  If no public key specified, InsecureIgnoreHostKey is assumed
	ServerPublicKey string `yaml:"server_public_key,omitempty" json:"server_public_key,omitempty"`
	// IgnoreDeadlines tells the tunnel to ignore deadline requests as the ssh tunnel does not implement
	IgnoreDeadlines bool `yaml:"ignore_deadlines,omitempty" json:"ignore_deadlines,omitempty"`
	// a map of ConnDefinitions for each db connection using the tunnel.  Each dsn will return a corresponding *sql.DB
	Datasources map[string]Datasource `yaml:"datasources,omitempty" json:"datasources,omitempty"`

	// database connection list with mutex for protection
	m     sync.Mutex
	dbMap map[string]*sql.DB
}

// ConfigError used to describe errors when opening
// DBs based upon a Config
type ConfigError struct {
	Msg        string
	Idx        int
	Addr       string
	DriverName string
	DSN        string
	Err        error
}

// Unwrap returns the internal error
func (ce *ConfigError) Unwrap() error {
	return ce.Err
}

// Error make the ConfigError an error
func (ce *ConfigError) Error() string {
	return fmt.Sprintf("%s %s: [%d]%s", ce.Addr, ce.DriverName, ce.Idx, ce.Msg)
}

func (ce *ConfigError) setErr(err error) *ConfigError {
	cx := *ce
	cx.Err = err
	return &cx
}

func (tc *TunnelConfig) newErr(idx int, dsn, msg string) *ConfigError {
	return &ConfigError{
		Addr: tc.HostPort,
		DSN:  dsn,
		Msg:  msg,
		Idx:  idx,
	}

}

// sshClientConfig validates values within the TunnelConfig and
// returns a ClientConfig that will be used for future db connections
func (tc *TunnelConfig) sshClientConfig() (*ssh.ClientConfig, error) {
	cfg := &ssh.ClientConfig{
		User:            tc.UserID,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if tc.Pwd > "" {
		cfg.Auth = append(cfg.Auth, ssh.Password(tc.Pwd))
	}
	var keybytes []byte
	if tc.ClientKeyFile > "" {
		filebytes, err := ioutil.ReadFile(tc.ClientKeyFile)
		if err != nil {
			return nil, tc.newErr(4, "", fmt.Sprintf("unable to open key file %s", tc.ClientKeyFile))
		}
		keybytes = filebytes
	}
	if tc.ClientKey > "" {
		keybytes = []byte(tc.ClientKey)
	}
	if len(keybytes) > 0 {
		key, err := parseKey([]byte(keybytes), tc.ClientKeyPwd)
		if err != nil {
			return nil, tc.newErr(5, "", fmt.Sprintf("key parse failed err: %v", err)).setErr(err)
		}
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(key))
	}

	hostKeyCallback, err := tc.getPublicKey()
	if err != nil {
		return nil, err
	}
	cfg.HostKeyCallback = hostKeyCallback

	return cfg, nil
}

func (tc *TunnelConfig) getPublicKey() (ssh.HostKeyCallback, error) {
	var pubkeybytes []byte
	if tc.ServerPublicKeyFile > "" {
		filebytes, err := ioutil.ReadFile(tc.ServerPublicKeyFile)
		if err != nil {
			return nil, tc.newErr(7, "", fmt.Sprintf("unable to open key file %s", tc.ClientKeyFile))
		}
		pubkeybytes = filebytes
	}
	if tc.ServerPublicKey > "" {
		pubkeybytes = []byte(tc.ServerPublicKey)
	}
	if len(pubkeybytes) > 0 {
		pk, err := parsePubKey(pubkeybytes)
		if err != nil {
			return nil, tc.newErr(8, "", fmt.Sprintf("pubkey parse failed err: %v", err)).setErr(err)
		}
		return ssh.FixedHostKey(pk), nil
	}
	return nil, nil
}

func (tc *TunnelConfig) validate() error {
	if tc.HostPort == "" {
		return tc.newErr(0, "", "address may not be blank")
	}
	if tc.UserID == "" {
		return tc.newErr(1, "", "user not specified")
	}
	if tc.ClientKey+tc.ClientKeyFile+tc.Pwd == "" {
		return tc.newErr(2, "", "no authenticate methods specified")
	}
	if tc.ClientKey > "" && tc.ClientKeyFile > "" {
		return tc.newErr(3, "", "may not specify a key and a key file")
	}
	if tc.ServerPublicKeyFile > "" && tc.ServerPublicKey > "" {
		return tc.newErr(6, "", "may not specify a server public key and a server public key file")
	}
	if len(tc.Datasources) == 0 {
		return tc.newErr(20, "", "at least one dsn string must be specified for tc.HostPort")
	}
	return nil
}

// DB returns an open DB based up the datasource defined by the name
// in the TunnelConfig
func (tc *TunnelConfig) DB(dbname string) (*sql.DB, error) {

	mp, err := tc.DatabaseMap()
	if err != nil {
		return nil, err
	}
	db, ok := mp[dbname]
	if !ok {
		return nil, tc.newErr(21, "", fmt.Sprintf("no database with name %s found in TunnelConfig", dbname))
	}
	return db, nil
}

// DatabaseMap returns *sql.DBs returns a map of *sql.DBs based upon
// the DatabaseMap field. Either all dbs defined in the config are
// returned with no error or no db is returned if an error occurs.
// Tunnels datasources connect in a lazy fashion so that the connections
// are not until a database command is called.
func (tc *TunnelConfig) DatabaseMap() (map[string]*sql.DB, error) {
	tc.m.Lock()
	defer tc.m.Unlock()
	if tc.dbMap != nil {
		return tc.dbMap, nil
	}

	if err := tc.validate(); err != nil {
		return nil, err
	}
	cfg, err := tc.sshClientConfig()
	if err != nil {
		return nil, err
	}

	tun, err := New(cfg, tc.HostPort)
	if err != nil {
		return nil, tc.newErr(9, "", fmt.Sprintf("new tunnel error: %v", err)).setErr(err)
	}
	tun.IgnoreSetDeadlineRequest(tc.IgnoreDeadlines)
	tc.dbMap = make(map[string]*sql.DB)

	for nm, dataSource := range tc.Datasources {
		dsn := dataSource.ConnectionString
		if dsn == "" {
			tc.closeDBs(tun)
			return nil, tc.newErr(13, dsn, fmt.Sprintf("%s db has empty datasourcename", nm))
		}
		tunnelDriver, err := dataSource.Driver()
		if err != nil {
			return nil, tc.newErr(12, dsn, fmt.Sprintf("[%s] invalid driver %s - %v", nm, dataSource.DriverName, err)).setErr(err)
		}
		sqlconn, err := tun.OpenConnector(tunnelDriver, dsn)
		if err != nil {
			tc.closeDBs(tun)
			return nil, tc.newErr(10, dsn, fmt.Sprintf("[%s] %s openconnector error: %v", nm, dataSource.DriverName, err)).setErr(err)
		}
		tc.dbMap[nm] = sql.OpenDB(sqlconn)
	}
	return tc.dbMap, nil
}

func (tc *TunnelConfig) closeDBs(tun *Tunnel) {
	for _, db := range tc.dbMap {
		db.Close()
	}
	tun.Close()
}
