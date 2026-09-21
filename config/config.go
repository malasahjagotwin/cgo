// Package config mengelola seluruh argument/flag CLI di satu tempat.
// Tambah atau ubah opsi cukup di file ini.
package config

import (
	"flag"
	"fmt"
	"os"
)

// Config menampung semua opsi yang bisa disesuaikan lewat argument CLI.
type Config struct {
	Host        string // alamat bind, mis. "0.0.0.0" atau "127.0.0.1"
	Port        string // port listen SSH (WAJIB, lewat -p)
	UsersFile   string // path file kredensial JSON
	HostKeyFile string // path private host key SSH
	Hostname    string // nama host yang tampil di prompt [user@<hostname>]
}

// Address mengembalikan alamat listen gabungan host:port.
func (c *Config) Address() string {
	return c.Host + ":" + c.Port
}

// Parse membaca argument CLI dan memvalidasinya.
// -p (port) wajib; tanpa itu program berhenti dengan exit code 1.
func Parse() (*Config, error) {
	c := &Config{}

	fs := flag.NewFlagSet("cnc", flag.ContinueOnError)
	fs.StringVar(&c.Port, "p", "", "port listen SSH server (WAJIB)")
	fs.StringVar(&c.Host, "host", "0.0.0.0", "alamat bind server")
	fs.StringVar(&c.UsersFile, "users", "user.json", "path file kredensial JSON")
	fs.StringVar(&c.HostKeyFile, "hostkey", "keys/host_key", "path private host key SSH")
	fs.StringVar(&c.Hostname, "hostname", "localhost", "nama host pada prompt")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "penggunaan: %s -p <port> [opsi]\n\nopsi:\n", os.Args[0])
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	if c.Port == "" {
		fs.Usage()
		return nil, fmt.Errorf("flag -p (port) wajib diisi")
	}

	return c, nil
}
