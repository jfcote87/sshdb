// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb

import (
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"

	"golang.org/x/crypto/ssh"
)

// TunnelConfig holds parameters for a ssh.ClientConfig for
// creating a ssh connection to the remote server define by
// the HostPort field.
type TunnelConfig struct {
	// address of remote server must be in the form "host:port", "host%zone:port",
	// "[host]:port" or "[host%zone]:port".  See func net.Dial for a description of
	// the hostport parameter.
	HostPort string `yaml:"hostport,omitempty" json:"hostport,omitempty"`
	// login nam for the remote ssh connection
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
	// list of dsn strings for db connection via the tunnel.  Each dsn will return a corresponding *sql.DB
	DSNList []string `yaml:"dsn_list,omitempty" json:"dsn_list,omitempty"`
}

// Config is a list of TunnelConfig records which returns prepared *sql.DBs
// using it's OpenDBs method.
type Config []TunnelConfig

// DBList returns a slice of db definitions base upon
// the config.  The order returned will match the dbs
// created in OpenDBs.
func (c Config) DBList() []string {
	var listing []string
	for _, tc := range c {
		addr := tc.HostPort
		for _, dsn := range tc.DSNList {
			listing = append(listing, addr+": "+dsn)
		}
	}
	return listing
}

// OpenDBs returns *sql.DBs returns a slice of *sql.DBs based upon
// the sequential dsn strings in the TunnelConfigs.  Use DBList to
// create a slice of matching descriptions for comparison.  Either all
// dbs defined in the config are returned with no error or no dbs
// are returned if an error occurs.
func (c Config) OpenDBs(tunnelDriver Driver) ([]*sql.DB, error) {
	if tunnelDriver == nil {
		return nil, errors.New("tunnel driver may not be nil")
	}
	var successFl = false
	var cfgdbs []*sql.DB

	defer func() {
		if !successFl { // if error close new DBs
			closeDBs(cfgdbs)
		}
	}()

	for _, tcfg := range c {
		if len(tcfg.DSNList) == 0 {
			return nil, fmt.Errorf("at least one dsn string must be specified for %s", tcfg.HostPort)
		}
		dbs, err := tcfg.openDBs(tunnelDriver)
		if err != nil {
			return nil, err
		}
		cfgdbs = append(cfgdbs, dbs...)
	}
	successFl = true
	return cfgdbs, nil
}

// ConfigError used to describe errors when opening
// DBs based upon a Config.
type ConfigError struct {
	Msg        string
	Idx        int
	Addr       string
	DriverName string
	DSN        string
	Err        error
}

// Unwrap returns the attached error.
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

func (tc *TunnelConfig) newErr(idx int, td Driver, dsn, msg string) *ConfigError {
	return &ConfigError{
		Addr:       tc.HostPort,
		DriverName: td.Name(),
		DSN:        dsn,
		Msg:        msg,
		Idx:        idx,
	}

}

func (tc *TunnelConfig) sshClientConfig(td Driver) (*ssh.ClientConfig, error) {
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
			return nil, tc.newErr(4, td, "", fmt.Sprintf("unable to open key file %s", tc.ClientKeyFile))
		}
		keybytes = filebytes
	}
	if tc.ClientKey > "" {
		keybytes = []byte(tc.ClientKey)
	}
	if len(keybytes) > 0 {
		key, err := parseKey([]byte(keybytes), tc.ClientKeyPwd)
		if err != nil {
			return nil, tc.newErr(5, td, "", fmt.Sprintf("key parse failed err: %v", err)).setErr(err)
		}
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(key))
	}
	var pubkeybytes []byte
	if tc.ServerPublicKeyFile > "" {
		filebytes, err := ioutil.ReadFile(tc.ServerPublicKeyFile)
		if err != nil {
			return nil, tc.newErr(7, td, "", fmt.Sprintf("unable to open key file %s", tc.ClientKeyFile))
		}
		pubkeybytes = filebytes
	}
	if tc.ServerPublicKey > "" {
		pubkeybytes = []byte(tc.ServerPublicKey)
	}
	if len(pubkeybytes) > 0 {
		pk, err := parsePubKey(pubkeybytes)
		if err != nil {
			return nil, tc.newErr(8, td, "", fmt.Sprintf("pubkey parse failed err: %v", err)).setErr(err)
		}
		cfg.HostKeyCallback = ssh.FixedHostKey(pk)
	}
	return cfg, nil

}

func (tc *TunnelConfig) validate(td Driver) error {
	if tc.HostPort == "" {
		return tc.newErr(0, td, "", "address may not be blank")
	}
	if tc.UserID == "" {
		return tc.newErr(1, td, "", "user not specified")
	}
	if tc.ClientKey == "" && tc.ClientKeyFile == "" && tc.Pwd == "" {
		return tc.newErr(2, td, "", "no authenticate methods specified")
	}
	if tc.ClientKey > "" && tc.ClientKeyFile > "" {
		return tc.newErr(3, td, "", "may not specify a key and a key file")
	}
	if tc.ServerPublicKeyFile > "" && tc.ServerPublicKey > "" {
		return tc.newErr(6, td, "", "may not specify a server public key and a server public key file")
	}
	return nil
}

func (tc *TunnelConfig) openDBs(tunnelDriver Driver) ([]*sql.DB, error) {
	if err := tc.validate(tunnelDriver); err != nil {
		return nil, err
	}
	cfg, err := tc.sshClientConfig(tunnelDriver)
	if err != nil {
		return nil, err
	}

	tun, err := New(tunnelDriver, cfg, tc.HostPort)
	if err != nil {
		return nil, tc.newErr(9, tunnelDriver, "", fmt.Sprintf("new tunnel error: %v", err)).setErr(err)
	}
	var dbs []*sql.DB

	for _, dsn := range tc.DSNList {
		sqlconn, err := tun.OpenConnector(dsn)
		if err != nil {
			closeDBs(dbs)
			tun.Close()
			return nil, tc.newErr(10, tunnelDriver, dsn, fmt.Sprintf("openconnector error: %v", err)).setErr(err)
		}
		dbs = append(dbs, sql.OpenDB(sqlconn))
	}
	return dbs, nil
}

func closeDBs(dblist []*sql.DB) {
	for i := range dblist {
		dblist[i].Close()
	}
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
