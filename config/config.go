package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	Host        string
	Port        string
	UsersFile   string
	HostKeyFile string
	Hostname    string
}

func (c *Config) Address() string {
	return c.Host + ":" + c.Port
}

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
