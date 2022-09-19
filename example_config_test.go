//Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb_test

import (
	"fmt"
	"log"

	"github.com/jfcote87/sshdb"
	"gopkg.in/yaml.v3"

	_ "github.com/jfcote87/sshdb/mssql"
	_ "github.com/jfcote87/sshdb/mysql"
)

// ExampleConfig demonstrates how to create tunnels and database
// connections via the Config/TunnelConfig structs
func ExampleConfig() {
	var cfg_yaml00 = `
# tunnel with a single connecton and key authentication
hostport: firewall.example.com:22
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
datasources:
  localsock: 
	driver_name: "mysql"
	dsn: login:passwd@unix(/tmp/my.sock)/dbname?parseTime=true
  local:
	driver_name: "mssql"
	dsn: uid=me;password=xpwd;server=localhost;database=crm
  exampledb: 
  	driver_name: "mysql"
  	dsn: admin:goodpwd@tcp(db.example.com:3601)/contacts?parseTime=true
`
	var config sshdb.TunnelConfig
	if err := yaml.Unmarshal([]byte(cfg_yaml00), &config); err != nil {
		log.Fatalf("yaml decode failed: %v", err)
	}
	dbs, err := config.DatabaseMap()
	if err != nil {
		log.Fatalf("opendbs fail: %v", err)
	}
	for nm, db := range dbs {
		msg := "ping success"
		if err := db.Ping(); err != nil {
			msg = fmt.Sprintf("%v", err)
		}
		log.Printf("%s %s", nm, msg)
		defer db.Close()
	}
}
