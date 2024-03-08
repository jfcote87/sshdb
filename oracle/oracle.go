const driverName = "oracle"

var TunnelDriver sshdb.Driver = tunnelDriver(driverName)

// OpenConnector returns a new oracle connector that uses the dialer to open ssh channel connections
// as the underlying network connections
func (tun tunnelDriver) OpenConnector(dialer sshdb.Dialer, dsn string) (driver.Connector, error) {

	oc := new(go_ora.OracleDriver)
	connector, err := oc.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}

	oConnector, ok := connector.(*go_ora.OracleConnector)
	if !ok {
		fmt.Println(err)

	}

	var newDialer go_network.DialerContext = dialer

	oConnector.Dialer(newDialer)

	return oConnector, nil
}

type tunnelDriver string

func (tun tunnelDriver) Name() string {
	return string(tun)
}
