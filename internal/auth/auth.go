// Package auth memuat kredensial dari file JSON dan memverifikasi login.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
)

// User merepresentasikan satu kredensial dari file users JSON.
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Store menampung daftar user dan menyediakan verifikasi login.
type Store struct {
	users []User
}

// Load membaca file JSON dan mengembalikan Store.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("baca %s: %w", path, err)
	}
	var users []User
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &Store{users: users}, nil
}

// Authenticate mengembalikan true bila username+password cocok.
func (s *Store) Authenticate(username, password string) bool {
	for _, u := range s.users {
		if u.Username == username && u.Password == password {
			return true
		}
	}
	return false
}

// Count mengembalikan jumlah user yang termuat.
func (s *Store) Count() int {
	return len(s.users)
}
