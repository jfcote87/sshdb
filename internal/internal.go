// Package internal contains a LoadTunnelConfig
// function that reads either a json or yaml representation of a sshdb.TunnelConfig
package internal

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/jfcote87/sshdb"
	"gopkg.in/yaml.v3"
)

// LoadTunnelConfig reads either a json or yaml representation of
// a sshdb.TunnelConfig
func LoadTunnelConfig(fn string) (*sshdb.TunnelConfig, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tunnelConfig *sshdb.TunnelConfig
	parts := strings.Split(fn, ".")
	switch parts[len(parts)-1] {
	case "yaml", "yml":
		if err := yaml.NewDecoder(f).Decode(&tunnelConfig); err != nil {
			return nil, err
		}
	case "json":
		if err := json.NewDecoder(f).Decode(&tunnelConfig); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("file must end with either .json, .yaml, or .yml")
	}
	return tunnelConfig, nil
}
