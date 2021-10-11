//Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb_test

import (
	"log"

	"github.com/jfcote87/sshdb"
	"github.com/jfcote87/sshdb/mysql"
	"gopkg.in/yaml.v3"
)

// ExampleConfig demonstrates how to create tunnels and database
// connections via the Config/TunnelConfig structs
func ExampleConfig() {
	var cfg_yaml = `
# tunnel with 2 db connections, passoword authentication
- hostport: ssh.example.com:22
  user_id: me
  pwd: best_password_ever
  dsn_list:
	- mylogin:sqlpass@tcp(localhost:3600)/schemaname?parseTime=true
	- admin:goodpwd@tcp(db.example.com:3601)/contacts?parseTime=true

# tunnel with a single connecton and key authentication
- hostport: firewall.example.com:22
  user_id: jfcote87
  client_key: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAArAAAABNlY2RzYS
    1zaGEyLW5pc3RwNTIxAAAACG5pc3RwNTIxAAAAhQQB98RdfbLOuKmtf874FnMEuVJhPF5c
    r8NdVV+4U4oeA42OgIb0SfnTTpmAVjE64MWsT96hRrb9ZTzDbk/7W5NGKNEAO+usYZ1X2f
    /E/a86vG11lZRx9HZXuVccJJCzqQlX0UQMzdYtk4pGuQojGXkei+WGpLfFBIVpjKZ+0A6g
    VKJ+2ogAAAEQagV3GWoFdxkAAAATZWNkc2Etc2hhMi1uaXN0cDUyMQAAAAhuaXN0cDUyMQ
    AAAIUEAffEXX2yzriprX/O+BZzBLlSYTxeXK/DXVVfuFOKHgONjoCG9En5006ZgFYxOuDF
    rE/eoUa2/WU8w25P+1uTRijRADvrrGGdV9n/xP2vOrxtdZWUcfR2V7lXHCSQs6kJV9FEDM
    3WLZOKRrkKIxl5HovlhqS3xQSFaYymftAOoFSiftqIAAAAQgCuV4B+Cak9BWL3vLW1Knb0
    R69k2oaTNn8ipqaI/X9MNbKwFb+O5a51nhRFsCzP3pd2awmGdx7hPkXH10YnlRDrvAAAAA
    1qY290ZUBKRkMtU01HAQIDBAU=
    -----END OPENSSH PRIVATE KEY-----
  public_server_key: cfg/pubkey.pub
  dsn_list:
    -- login:passwd@unix(/tmp/my.sock)/dbname?parseTime=true
`
	var config sshdb.Config
	if err := yaml.Unmarshal([]byte(cfg_yaml), &config); err != nil {
		log.Fatalf("yaml decode failed: %v", err)
	}
	dbs, err := config.OpenDBs(mysql.TunnelDriver)
	if err != nil {
		log.Fatalf("opendbs fail: %v", err)
	}
	dbtags := config.DBList()
	for i, db := range dbs {
		if err := db.Ping(); err != nil {
			log.Printf("%s %v", dbtags[i], err)
		}
		db.Close()
	}
}
