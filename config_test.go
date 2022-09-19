// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/jfcote87/sshdb"
	"gopkg.in/yaml.v3"
)

func TestTunnelConfig(t *testing.T) {
	sshdb.RegisterDriver("test_driver", testDriver)
	tests := []struct {
		name         string
		hasErr       bool
		errIdx       int
		tunnelDrvIdx int
		numDB        int
	}{
		{name: "fail00", hasErr: true, errIdx: 0},
		{name: "fail01", hasErr: true, errIdx: 0},
		{name: "fail02", hasErr: true, errIdx: 1},
		{name: "fail03", hasErr: true, errIdx: 20},
		{name: "success04", numDB: 1},
		{name: "fail05", hasErr: true, errIdx: 2},
		{name: "fail06", hasErr: true, errIdx: 3},
		{name: "fail07", hasErr: true, errIdx: 4},
		{name: "fail08", hasErr: true, errIdx: 5},
		{name: "success09", numDB: 1},
		{name: "success10", numDB: 1},
		{name: "fail11", hasErr: true, errIdx: 6},
		{name: "fail12", hasErr: true, errIdx: 7},
		{name: "fail13", hasErr: true, errIdx: 8},
		{name: "success14", numDB: 1},
		{name: "fail15", hasErr: true, errIdx: 9},
		{name: "fail16", hasErr: true, errIdx: 12},
		{name: "fail17", hasErr: true, errIdx: 10},
		{name: "fail18", hasErr: true, errIdx: 13},
		{name: "success19", numDB: 3},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := fmt.Sprintf("testfiles/config/test%02d.yaml", i)
			cfg, err := getTunnelConfig(fn)
			if err != nil {
				t.Errorf("config file load failed %s: %v", fn, err)
				return
			}
			dbs, err := cfg.DatabaseMap()
			if tt.hasErr {
				var ce *sshdb.ConfigError
				if !errors.As(err, &ce) {
					t.Errorf("%s expected ConfigError %d; got %v", tt.name, tt.errIdx, err)
					return
				}
				if ce.Idx != tt.errIdx {
					t.Errorf("%s expected ConfigError %d; got %d %v", tt.name, tt.errIdx, ce.Idx, err)
				}
				return
			}
			if err != nil {
				t.Errorf("%s expected success; got %v", tt.name, err)
			}
			if len(dbs) != tt.numDB {
				t.Errorf("%s expected %d dbs; got %d", tt.name, tt.numDB, len(dbs))
			}
		})
	}
}

func getTunnelConfig(fn string) (*sshdb.TunnelConfig, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg *sshdb.TunnelConfig
	switch fn[len(fn)-4:] {
	case "yaml":
		return cfg, yaml.NewDecoder(f).Decode(&cfg)
	case "json":
		return cfg, json.NewDecoder(f).Decode(&cfg)
	}
	return nil, fmt.Errorf("invalid type %s", fn[len(fn)-4:])
}

func TestTunnelConfig_DBList(t *testing.T) {
	fn := "testfiles/config/test19.yaml"
	config, err := getTunnelConfig(fn)
	if err != nil {
		t.Errorf("config file load failed %s: %v", fn, err)
		return
	}
	cfgtags := config.Datasources
	tt := map[string]string{
		"valid00": "valid_dsn_string",
		"valid01": "valid_dsn_string2",
		"valid02": "valid_dsn_string3",
	}
	if len(tt) != len(cfgtags) {
		t.Errorf("expected %d recs; got %d", len(tt), len(cfgtags))
	}
	for k, v := range tt {
		if v != cfgtags[k].ConnectionString {
			t.Errorf("expected %s with name %s; got %s", v, k, cfgtags[k].ConnectionString)
		}
	}

}

func TestConfigError(t *testing.T) {
	ce := &sshdb.ConfigError{
		Msg:        "error msg",
		Idx:        99,
		Addr:       "ssh.example.com:22",
		DriverName: "testDriver",
	}
	xerr := fmt.Errorf("TEST %w", ce)
	if errors.Is(ce, io.EOF) {
		t.Errorf("expected no io.EOF from unwrap")
	}
	ce.Err = io.EOF
	if !errors.Is(ce, io.EOF) {
		t.Error("expected io.EOF from unwrap")
	}
	expectedMsg := "TEST ssh.example.com:22 testDriver: [99]error msg"
	if xerr.Error() != expectedMsg {
		t.Errorf("expected msg %s; got %s", expectedMsg, xerr.Error())
	}
}
