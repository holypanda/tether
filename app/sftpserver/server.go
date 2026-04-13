package sftpserver

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"tether/identity"
)

type Config struct {
	RootDir          string
	HostIdentity     *identity.Identity
	AllowedClientKey ssh.PublicKey
}

type Server struct {
	listener  net.Listener
	cfg       Config
	done      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
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
	s.closeOnce.Do(func() {
		close(s.done)
		s.closeErr = s.listener.Close()
	})
	s.wg.Wait()
	return s.closeErr
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
					handlers := &chrootHandlers{root: s.cfg.RootDir}
					sftpHandlers := sftp.Handlers{
						FileGet:  handlers,
						FilePut:  handlers,
						FileCmd:  handlers,
						FileList: handlers,
					}
					srv := sftp.NewRequestServer(ch, sftpHandlers)
					_ = srv.Serve()
					srv.Close()
					return
				}
				_ = req.Reply(ok, nil)
			}
		}(ch, reqs)
	}
}

// chrootHandlers enforces that all SFTP operations stay within root.
type chrootHandlers struct {
	root string
}

func (c *chrootHandlers) realPath(p string) (string, error) {
	clean := filepath.Clean("/" + strings.TrimPrefix(p, "/"))
	full := filepath.Join(c.root, clean)
	rel, err := filepath.Rel(c.root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", os.ErrPermission
	}
	return full, nil
}

func (c *chrootHandlers) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(full, os.O_RDONLY, 0)
}

func (c *chrootHandlers) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if !r.Pflags().Append {
		flags |= os.O_TRUNC
	}
	return os.OpenFile(full, flags, 0o644)
}

func (c *chrootHandlers) Filecmd(r *sftp.Request) error {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return err
	}
	switch r.Method {
	case "Setstat":
		return nil
	case "Rename":
		target, err := c.realPath(r.Target)
		if err != nil {
			return err
		}
		return os.Rename(full, target)
	case "Rmdir":
		return os.Remove(full)
	case "Mkdir":
		return os.Mkdir(full, 0o755)
	case "Remove":
		return os.Remove(full)
	case "Symlink", "Link":
		return os.ErrPermission
	}
	return nil
}

type listerAt []os.FileInfo

func (l listerAt) ListAt(f []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}
	n := copy(f, l[offset:])
	if n < len(f) {
		return n, io.EOF
	}
	return n, nil
}

func (c *chrootHandlers) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	full, err := c.realPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case "List":
		entries, err := os.ReadDir(full)
		if err != nil {
			return nil, err
		}
		infos := make([]os.FileInfo, 0, len(entries))
		for _, e := range entries {
			info, err := e.Info()
			if err == nil {
				infos = append(infos, info)
			}
		}
		return listerAt(infos), nil
	case "Stat":
		info, err := os.Stat(full)
		if err != nil {
			return nil, err
		}
		return listerAt([]os.FileInfo{info}), nil
	}
	return nil, os.ErrInvalid
}
