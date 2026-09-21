// Package server menjalankan SSH server: listen, terima koneksi,
// autentikasi, lalu serahkan tiap sesi ke package shell.
package server

import (
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"

	"github.com/malasahjagotwin/cnc/config"
	"github.com/malasahjagotwin/cnc/internal/auth"
	"github.com/malasahjagotwin/cnc/internal/hostkey"
	"github.com/malasahjagotwin/cnc/internal/prompt"
	"github.com/malasahjagotwin/cnc/internal/shell"
)

// Server menampung dependensi runtime SSH server.
type Server struct {
	cfg    *config.Config
	users  *auth.Store
	theme  prompt.Theme
	sshCfg *ssh.ServerConfig
}

// New membangun Server dari konfigurasi: memuat user & host key.
func New(cfg *config.Config) (*Server, error) {
	users, err := auth.Load(cfg.UsersFile)
	if err != nil {
		return nil, err
	}

	signer, err := hostkey.LoadOrCreate(cfg.HostKeyFile)
	if err != nil {
		return nil, fmt.Errorf("host key: %w", err)
	}

	s := &Server{
		cfg:   cfg,
		users: users,
		theme: prompt.DefaultTheme,
	}

	s.sshCfg = &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if s.users.Authenticate(c.User(), string(pass)) {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("kredensial salah untuk %q", c.User())
		},
	}
	s.sshCfg.AddHostKey(signer)

	return s, nil
}

// ListenAndServe mulai listen dan melayani koneksi (blocking).
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Address())
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Address(), err)
	}
	fmt.Printf("SSH server berjalan di %s (%d user termuat)\n", s.cfg.Address(), s.users.Count())

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(nConn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, s.sshCfg)
	if err != nil {
		nConn.Close()
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	username := sshConn.User()
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			newCh.Reject(ssh.UnknownChannelType, "hanya session yang didukung")
			continue
		}
		channel, requests, err := newCh.Accept()
		if err != nil {
			continue
		}

		go func(in <-chan *ssh.Request) {
			for req := range in {
				switch req.Type {
				case "shell", "pty-req":
					req.Reply(true, nil)
				default:
					req.Reply(false, nil)
				}
			}
		}(requests)

		go shell.New(channel, s.theme, username, s.cfg.Hostname).Run()
	}
}
