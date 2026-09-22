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
	fs.StringVar(&c.Port, "p", "", "SSH server listen port (REQUIRED)")
	fs.StringVar(&c.Host, "host", "0.0.0.0", "server bind address")
	fs.StringVar(&c.UsersFile, "users", "user.json", "path to users JSON credentials file")
	fs.StringVar(&c.HostKeyFile, "hostkey", "keys/host_key", "path to SSH private host key")
	fs.StringVar(&c.Hostname, "hostname", "localhost", "hostname shown in the prompt")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s -p <port> [options]\n\noptions:\n", os.Args[0])
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
	}

	if c.Port == "" {
		fs.Usage()
		return nil, fmt.Errorf("flag -p (port) is required")
	}

	return c, nil
}
