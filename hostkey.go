package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"

	"golang.org/x/crypto/ssh"
)

// loadOrCreateHostKey memuat host key dari path, atau membuat & menyimpan
// key ed25519 baru jika belum ada.
func loadOrCreateHostKey(path string) (ssh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(data)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		return nil, err
	}

	return ssh.NewSignerFromSigner(priv)
}
