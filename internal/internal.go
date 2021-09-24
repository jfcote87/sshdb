// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package internal provide utility funcs for the sshdb package
package internal

import (
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/jfcote87/sshdb"
	"golang.org/x/crypto/ssh"
)

// ErrNoEnvVariable is returned by GetClientConfig if SSHDB_DSN is empty
var ErrNoEnvVariable = errors.New("no env variable")

// DBFromEnvSettings returns an ssh client config and other connection parameters
// based upon enviromental variariables.
// SSHDB_CONNECTION=host:port,username,password,client_private_key_filepath,private_key_password,server_public_key_filepath
func DBFromEnvSettings(opener sshdb.ConnectorOpener) (*sql.DB, error) {
	if opener == nil {
		return nil, errors.New("opener may not be nil")
	}
	settings := os.Getenv("SSHDB_CLIENT_CONNECTION_" + strings.ToUpper(opener.Name()))
	if settings == "" {
		return nil, ErrNoEnvVariable
	}
	parts := strings.SplitN(settings, ",", 7)
	for len(parts) < 7 {
		parts = append(parts, "")
	}
	sshaddr, user, pwd, cert, certpwd, servercert, dsn := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]
	if user == "" {
		return nil, errors.New("user not specified")
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	if cert > "" {
		key, err := getKey(cert, certpwd)
		if err != nil {
			return nil, err
		}
		cfg.Auth = append(cfg.Auth, ssh.PublicKeys(key))
	}
	if pwd > "" {
		cfg.Auth = append(cfg.Auth, ssh.Password(pwd))
	}
	if servercert > "" {
		pubkey, err := getPubKey(servercert)
		if err != nil {
			return nil, err
		}
		cfg.HostKeyCallback = ssh.FixedHostKey(pubkey)
	}
	tunnel, err := sshdb.New(opener, cfg, sshaddr)
	if err != nil {
		return nil, fmt.Errorf("unable to open %s tunnel (%s) %v", opener.Name(), sshaddr, err)
	}
	cn, err := tunnel.OpenConnector(dsn)
	if err != nil {
		return nil, fmt.Errorf("%s %s %v", err, opener.Name(), dsn)
	}
	return sql.OpenDB(cn), nil

}

func getKey(fn, pwd string) (ssh.Signer, error) {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("unable to read file %s %v", fn, err)
	}
	if pwd > "" {
		return ssh.ParsePrivateKeyWithPassphrase(b, []byte(pwd))
	}
	return ssh.ParsePrivateKey(b)
}

func getPubKey(fn string) (ssh.PublicKey, error) {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("unable to read public key file %s %v", fn, err)
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey(b)
	return k, err
}
