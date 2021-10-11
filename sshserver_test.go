// Copyright 2021 James Cote
// All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sshdb_test

import (
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

/* RFC4254 7.2 */
type directTCPPayload struct {
	Addr       string // To connect to
	Port       uint32
	OriginAddr string
	OriginPort uint32
}

func (p directTCPPayload) dial() (net.Conn, error) {
	c, err := net.Dial("tcp", fmt.Sprintf("[%s]:%d", p.Addr, p.Port))
	if err != nil {
		return nil, fmt.Errorf("unable to dial [%s]:%d %v", p.Addr, p.Port, err)
	}
	return c, nil

}

/* https://github.com/openssh/openssh-portable/blob/724eb900ace30661d45db2ba01d0f924d95ecccb/PROTOCOL#L235 */
type directStreamPayload struct {
	Socket      string
	Reserved    string
	ReservedInt uint32
}

func (p directStreamPayload) dial() (net.Conn, error) {
	c, err := net.Dial("unix", p.Socket)
	if err != nil {
		return nil, fmt.Errorf("unable to dial %s %v", p.Socket, err)
	}
	return c, nil
}

type payload interface {
	dial() (net.Conn, error)
}

type directTCPServer struct {
	signer ssh.Signer
	key    ssh.PublicKey
	userID string
	pwd    string
	addr   string
	laddr  []string
	srvcfg *ssh.ServerConfig

	//m  sync.Mutex
	wg sync.WaitGroup
}

func (d *directTCPServer) clientConfig() *ssh.ClientConfig {
	var method ssh.AuthMethod
	if d.key == nil {
		method = ssh.Password(d.pwd)
	} else {
		method = ssh.PublicKeys(d.signer)
	}
	_ = method
	return &ssh.ClientConfig{
		User: "me",
		Auth: []ssh.AuthMethod{
			method,
		},
		HostKeyCallback: ssh.FixedHostKey(d.signer.PublicKey()),
	}
}

type mockMysqlListener struct {
	l  net.Listener
	nm string
}

// ssh server able to handle direct-tcp connections only.  Authentication may be
// public key or password based (password always "nopassword")
// Much of the code is from https://gist.github.com/jpillora/b480fde82bff51a06238
func (d *directTCPServer) start() (func(), error) {
	// config := serverCfg(d.userID, d.key)
	config := d.srvcfg
	config.AddHostKey(d.signer)

	var mockListeners []mockMysqlListener
	for _, addr := range d.laddr {
		l, err := d.mockDBServer(addr)
		if err != nil {
			for _, ml := range mockListeners {
				ml.l.Close()
			}
			return nil, fmt.Errorf("unable to listen on %s - %v", addr, err)
		}
		mockListeners = append(mockListeners, mockMysqlListener{l: l, nm: addr})
	}
	// Once a ServerConfig has been configured, connections can be accepted.
	sshServerListener, err := net.Listen("tcp", d.addr)
	if err != nil {
		return nil, fmt.Errorf("sshserver failed to listen on %s, %v", d.addr, err)
	}
	// one for each mock and one for sshServerListener
	d.wg.Add(len(mockListeners) + 1)

	go func() {
		// Accept all connections
		for {
			socketConn, err := sshServerListener.Accept()
			if err != nil {
				d.wg.Done()
				break
			}
			// Before use, a handshake must be performed on the incoming net.Conn.
			sshConn, chans, reqs, err := ssh.NewServerConn(socketConn, config)
			if err != nil {
				continue
			}
			go func() {
				sshConn.Wait()
			}()
			// Discard all global out-of-band Requests
			go ssh.DiscardRequests(reqs)

			// Accept all channels
			go d.handleChannels(chans)
		}
	}()

	return func() {
		sshServerListener.Close()
		for _, ml := range mockListeners {
			_ = ml.l.Close()
		}
		d.wg.Wait()
	}, nil

}

func (d *directTCPServer) handleChannels(chans <-chan ssh.NewChannel) {
	// Service the incoming Channel channel in go routine
	for newChannel := range chans {
		go d.handleChannel(newChannel)
	}

}

func (d *directTCPServer) handleChannel(newChannel ssh.NewChannel) {
	// expect a channel type of "direct-tcpip"
	chType := newChannel.ChannelType()

	var payload payload
	switch chType {
	case "direct-tcpip":
		payload = &directTCPPayload{}
	case "direct-streamlocal@openssh.com":
		payload = &directStreamPayload{}
	default:
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("unknown channel type: %s", chType))
		return
	}
	if err := ssh.Unmarshal(newChannel.ExtraData(), payload); err != nil {
		newChannel.Reject(ssh.Prohibited, fmt.Sprintf("unmarshal payload error %v", err))
		return
	}

	rconn, err := payload.dial()
	if err != nil {
		newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}

	// accept connection to client channel
	connection, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)

	// Prepare teardown function
	close := func() {
		connection.Close()
		rconn.Close()
	}

	//pipe session between sockets
	var once sync.Once
	go func() {
		io.Copy(connection, rconn)
		once.Do(close)
	}()
	go func() {
		io.Copy(rconn, connection)
		once.Do(close)
	}()

}

func (d *directTCPServer) mockDBServer(laddr string) (net.Listener, error) {
	network := "tcp"
	if len(laddr) > 0 && laddr[0] == '/' {
		network = "unix"
	}
	l, err := net.Listen(network, laddr)
	if err != nil {
		return nil, fmt.Errorf("mockdb listening failed %v", err)
	}
	// Close the listener when the application closes.
	go func() {
		defer d.wg.Done()
		for i := 0; ; i++ {
			// Listen for an incoming connection.
			conn, err := l.Accept()
			if err != nil {
				break
			}
			// Handle connections in a new goroutine.
			defer conn.Close()
			go mockDBPingHandler(conn)
		}
	}()
	return l, nil
}

// Handles incoming requests.
func mockDBPingHandler(conn net.Conn) {
	defer conn.Close()

	// Make a buffer to hold incoming data.
	buffer := make([]byte, 128)
	bcnt := 0
	for {
		for bcnt < 128 {
			cnt, err := conn.Read(buffer[bcnt:])
			if err != nil {
				return
			}
			bcnt += cnt
		}
		bcnt = 0
		for bcnt < 128 {
			cnt, err := conn.Write(buffer[bcnt:])
			if err != nil {
				return
			}
			bcnt += cnt
		}
	}
}
