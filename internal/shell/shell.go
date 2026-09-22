// Package shell menyediakan shell interaktif per-sesi SSH.
//
// Menambah command baru: daftarkan di commandRegistry() di bawah.
package shell

import (
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/malasahjagotwin/cnc/internal/prompt"
)

// Command adalah satu perintah shell.
type Command struct {
	Name string
	Help string
	// Run mengeksekusi perintah. Kembalikan true untuk menutup sesi.
	Run func(s *Session, args []string) bool
}

// Session menampung state satu koneksi shell.
type Session struct {
	channel  ssh.Channel
	term     *term.Terminal
	theme    prompt.Theme
	username string
	hostname string
	commands map[string]Command
}

// New membuat Session baru di atas channel SSH.
func New(channel ssh.Channel, theme prompt.Theme, username, hostname string) *Session {
	t := term.NewTerminal(channel, "")
	s := &Session{
		channel:  channel,
		term:     t,
		theme:    theme,
		username: username,
		hostname: hostname,
	}
	s.commands = commandRegistry()
	t.SetPrompt(theme.Build(username, hostname))
	return s
}

// print menulis teks ke terminal klien (dengan CRLF).
func (s *Session) print(text string) {
	io.WriteString(s.channel, text+"\r\n")
}

// clear membersihkan layar sekaligus buffer scrollback klien.
func (s *Session) clear() {
	io.WriteString(s.channel, "\x1b[H\x1b[2J\x1b[3J")
}

// SetSize memberitahu line-editor ukuran terminal klien (kolom x baris).
// Wajib dipanggil dari pty-req/window-change agar navigasi kursor dan
// tombol panah berperilaku seperti SSH sungguhan.
func (s *Session) SetSize(width, height int) {
	if width > 0 && height > 0 {
		s.term.SetSize(width, height)
	}
}

// Run menjalankan loop baca-eksekusi sampai klien keluar.
func (s *Session) Run() {
	defer s.channel.Close()

	// Bersihkan layar + buffer scrollback agar sesi fresh dan
	// riwayat terminal sebelumnya tidak bisa digulir ke atas.
	s.clear()

	s.print(s.theme.Gradient("Selamat datang, " + s.username + "!"))
	s.print("ketik 'help' untuk daftar perintah")

	for {
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
			s.print("perintah tidak dikenal: " + fields[0])
			continue
		}
		if cmd.Run(s, fields[1:]) {
			return
		}
	}
}

// commandRegistry mendefinisikan semua perintah yang tersedia.
// Tambah perintah baru cukup di sini.
func commandRegistry() map[string]Command {
	cmds := map[string]Command{
		"help": {Name: "help", Help: "tampilkan daftar perintah", Run: cmdHelp},
		"whoami": {Name: "whoami", Help: "tampilkan user saat ini", Run: func(s *Session, _ []string) bool {
			s.print(s.theme.Gradient(s.username))
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
