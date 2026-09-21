package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// User merepresentasikan satu kredensial dari user.json.
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func loadUsers(path string) ([]User, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// gradientText mewarnai teks dengan gradasi merah -> abu-abu -> putih
// menggunakan 24-bit truecolor ANSI. Hanya teksnya yang diwarnai.
func gradientText(s string) string {
	// Titik awal (merah) dan titik akhir (putih), melewati abu-abu di tengah.
	start := [3]int{220, 40, 40}    // merah
	end := [3]int{255, 255, 255}    // putih
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range runes {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		red := int(float64(start[0]) + (float64(end[0])-float64(start[0]))*t)
		green := int(float64(start[1]) + (float64(end[1])-float64(start[1]))*t)
		blue := int(float64(start[2]) + (float64(end[2])-float64(start[2]))*t)
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", red, green, blue, r)
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

const (
	white = "\x1b[38;2;255;255;255m"
	reset = "\x1b[0m"
)

// buildPrompt menyusun prompt [username@localhost].
// Kurung [] dan @ berwarna putih, username & localhost bergradasi.
func buildPrompt(username string) string {
	return white + "[" + reset +
		gradientText(username) +
		white + "@" + reset +
		gradientText("localhost") +
		white + "]" + reset + " "
}

func main() {
	port := flag.String("p", "", "port untuk SSH server listen (wajib)")
	flag.Parse()

	if *port == "" {
		fmt.Fprintln(os.Stderr, "error: flag -p (port) wajib diisi")
		fmt.Fprintln(os.Stderr, "penggunaan: ./main -p <port>")
		os.Exit(1)
	}

	users, err := loadUsers("user.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: gagal membaca user.json: %v\n", err)
		os.Exit(1)
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			for _, u := range users {
				if u.Username == c.User() && u.Password == string(pass) {
					return &ssh.Permissions{}, nil
				}
			}
			return nil, fmt.Errorf("kredensial salah untuk user %q", c.User())
		},
	}

	signer, err := loadOrCreateHostKey("host_key")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: gagal menyiapkan host key: %v\n", err)
		os.Exit(1)
	}
	config.AddHostKey(signer)

	addr := ":" + *port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: gagal listen di port %s: %v\n", *port, err)
		os.Exit(1)
	}
	fmt.Printf("SSH server berjalan di port %s\n", *port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn, config)
	}
}

func handleConn(nConn net.Conn, config *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		nConn.Close()
		return
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	username := sshConn.User()

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "hanya session yang didukung")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}

		go func(in <-chan *ssh.Request) {
			for req := range in {
				switch req.Type {
				case "shell":
					req.Reply(true, nil)
				case "pty-req":
					req.Reply(true, nil)
				default:
					req.Reply(false, nil)
				}
			}
		}(requests)

		go runShell(channel, username)
	}
}

func runShell(channel ssh.Channel, username string) {
	defer channel.Close()

	t := term.NewTerminal(channel, "")
	t.SetPrompt(buildPrompt(username))

	welcome := gradientText("Selamat datang, "+username+"!") + "\r\n" +
		white + "Ketik 'help' untuk daftar perintah, 'exit' untuk keluar." + reset + "\r\n"
	io.WriteString(channel, welcome)

	for {
		line, err := t.ReadLine()
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch {
		case line == "exit" || line == "quit" || line == "logout":
			io.WriteString(channel, gradientText("Sampai jumpa!")+"\r\n")
			return
		case line == "help":
			io.WriteString(channel, white+"Perintah: help, whoami, clear, exit"+reset+"\r\n")
		case line == "whoami":
			io.WriteString(channel, gradientText(username)+"\r\n")
		case line == "clear":
			io.WriteString(channel, "\x1b[2J\x1b[H")
		default:
			io.WriteString(channel, "perintah tidak dikenal: "+line+"\r\n")
		}
	}
}
