package sftpserver

import (
	"bytes"
	"fmt"
	"net"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"stim-link/identity"
)

type Config struct {
	RootDir          string
	HostIdentity     *identity.Identity
	AllowedClientKey ssh.PublicKey
}

type Server struct {
	listener net.Listener
	cfg      Config
	done     chan struct{}
	wg       sync.WaitGroup
}

func Start(cfg Config) (*Server, error) {
	sshCfg := &ssh.ServerConfig{
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), cfg.AllowedClientKey.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("unauthorized key")
		},
	}
	hostSigner, err := cfg.HostIdentity.Signer()
	if err != nil {
		return nil, err
	}
	sshCfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{listener: ln, cfg: cfg, done: make(chan struct{})}
	go s.serve(sshCfg)
	return s, nil
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *Server) Close() error {
	close(s.done)
	err := s.listener.Close()
	s.wg.Wait()
	return err
}

func (s *Server) serve(sshCfg *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			return
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConn(c, sshCfg)
		}(conn)
	}
}

func (s *Server) handleConn(nConn net.Conn, sshCfg *ssh.ServerConfig) {
	defer nConn.Close()
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, sshCfg)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel")
			continue
		}
		ch, reqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
			defer ch.Close()
			for req := range reqs {
				ok := false
				if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
					ok = true
					_ = req.Reply(true, nil)
					srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(s.cfg.RootDir))
					if err != nil {
						return
					}
					_ = srv.Serve()
					return
				}
				_ = req.Reply(ok, nil)
			}
		}(ch, reqs)
	}
}
