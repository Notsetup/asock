package main

import (
	"crypto/sha1"
	"encoding/binary"
	"github.com/xtaci/kcp-go"
	"golang.org/x/crypto/pbkdf2"
	"io"
	"log"
	"net"
)

type ServerConfig struct {
	listen *net.UDPAddr
}

func NewServer(listenAddr string) (*ServerConfig, error) {
	listen, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, err
	}
	return &ServerConfig{listen}, nil
}

func (s *ServerConfig) Listen() error {
	buf := make([]byte, 256)
	key := pbkdf2.Key([]byte("demo pass"), []byte("demo salt"), 1024, 32, sha1.New)
	block, _ := kcp.NewAESBlockCrypt(key)
	listener, err := kcp.ListenWithOptions(s.listen.String(), block, 10, 3)
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		userConn, err := listener.AcceptKCP()
		if err != nil {
			log.Println(err)
			continue
		}
		userConn.SetStreamMode(true)
		go func(Conn *kcp.UDPSession) {
			defer Conn.Close()
			_, err := Conn.Read(buf)
			if err != nil || buf[0] != 5 {
				return
			}
			Conn.Write([]byte{5, 0})
			n, err := Conn.Read(buf)
			if err != nil || n < 7 || buf[1] != 1 {
				return
			}
			var dIP []byte
			switch buf[3] {
			case 0x01:
				dIP = buf[4 : 4+net.IPv4len]
			case 0x03:
				ipAddr, err := net.ResolveIPAddr("ip", string(buf[5:n-2]))
				if err != nil {
					return
				}
				dIP = ipAddr.IP
			case 0x04:
				dIP = buf[4 : 4+net.IPv6len]
			default:
				return
			}
			dPort := buf[n-2:]
			dstAddr := &net.TCPAddr{
				IP:   dIP,
				Port: int(binary.BigEndian.Uint16(dPort)),
			}
			dstServer, err := net.DialTCP("tcp", nil, dstAddr)
			if err != nil {
				return
			} else {
				defer dstServer.Close()
				dstServer.SetLinger(0)

				Conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
			}
			go func() {
				for {
					io.Copy(Conn, dstServer)
				}
				Conn.Close()
				dstServer.Close()
			}()
			for {
				io.Copy(dstServer, Conn)
			}
		}(userConn)
	}
}
