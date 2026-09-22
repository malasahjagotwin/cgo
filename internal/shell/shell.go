package shell

import (
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/malasahjagotwin/cnc/internal/auth"
	"github.com/malasahjagotwin/cnc/internal/prompt"
)

type Command struct {
	Name string
	Help string
	Run  func(s *Session, args []string) bool
}

type Session struct {
	channel  ssh.Channel
	term     *term.Terminal
	theme    prompt.Theme
	username string
	hostname string
	time     string
	slot     int
	cooldown int
	prompt   string
	commands map[string]Command
}

func New(channel ssh.Channel, theme prompt.Theme, user auth.User, hostname string) *Session {
	t := term.NewTerminal(channel, "")
	s := &Session{
		channel:  channel,
		term:     t,
		theme:    theme,
		username: user.Username,
		hostname: hostname,
		time:     user.Time,
		slot:     user.Slot,
		cooldown: user.Cooldown,
		prompt:   theme.Build(user.Username, hostname),
	}
	s.commands = commandRegistry()
	return s
}

func (s *Session) print(text string) {
	io.WriteString(s.channel, text+"\r\n")
}

func (s *Session) clear() {
	io.WriteString(s.channel, "\x1b[H\x1b[2J\x1b[3J")
}

func (s *Session) SetSize(width, height int) {
	if width > 0 && height > 0 {
		s.term.SetSize(width, height)
	}
}

func (s *Session) Run() {
	defer s.channel.Close()

	s.clear()

	for {
		io.WriteString(s.channel, s.prompt)

		line, err := s.term.ReadLine()
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		cmd, ok := s.commands[fields[0]]
		if !ok {
			continue
		}
		if cmd.Run(s, fields[1:]) {
			return
		}
	}
}

func commandRegistry() map[string]Command {
	cmds := map[string]Command{
		"help": {Name: "help", Help: "tampilkan daftar perintah", Run: cmdHelp},
		"whoami": {Name: "whoami", Help: "tampilkan user saat ini", Run: func(s *Session, _ []string) bool {
			s.print(s.theme.Gradient(s.username))
			s.print("Time     : " + s.time + "s")
			s.print("Slot     : " + strconv.Itoa(s.slot))
			s.print("Cooldown : " + strconv.Itoa(s.cooldown) + "s")
			return false
		}},
		"clear": {Name: "clear", Help: "bersihkan layar", Run: func(s *Session, _ []string) bool {
			s.clear()
			return false
		}},
		"echo": {Name: "echo", Help: "cetak kembali argumen", Run: func(s *Session, args []string) bool {
			s.print(strings.Join(args, " "))
			return false
		}},
	}
	exit := Command{Name: "exit", Help: "keluar dari sesi", Run: func(s *Session, _ []string) bool {
		s.print(s.theme.Gradient("Sampai jumpa!"))
		return true
	}}
	cmds["exit"] = exit
	cmds["quit"] = exit
	cmds["logout"] = exit
	return cmds
}

func cmdHelp(s *Session, _ []string) bool {
	seen := map[string]bool{}
	for _, c := range s.commands {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		s.print(s.theme.Gradient(c.Name) + "\x1b[0m — " + c.Help)
	}
	return false
}
