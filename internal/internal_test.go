package internal_test

import (
	"testing"

	"github.com/jfcote87/sshdb/internal"
)

func TestLoadTunnelConfig(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		//want    *sshdb.TunnelConfig
		wantErr bool
	}{
		{name: "t01", fn: "../testfiles/config/test19.yaml", wantErr: false},
		{name: "t02", fn: "../testfiles/config/test19.badformat.yaml", wantErr: true},
		{name: "t03", fn: "../testfiles/config/test19.json", wantErr: false},
		{name: "t04", fn: "../testfiles/config/test19.badformat.json", wantErr: true},
		{name: "t05", fn: "../testfiles/config/test19", wantErr: true},
		{name: "t06", fn: "../testfiles/doesnonexist", wantErr: true},
	}
	for idx, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := internal.LoadTunnelConfig(tt.fn)
			if (err != nil) != tt.wantErr {
				t.Errorf("%d LoadTunnelConfig() error = %v, wantErr %v", idx, err, tt.wantErr)
				return
			}
		})
	}
}
