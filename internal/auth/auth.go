package auth

import (
	"encoding/json"
	"fmt"
	"os"
)

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Store struct {
	users []User
}

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

func (s *Store) Authenticate(username, password string) bool {
	for _, u := range s.users {
		if u.Username == username && u.Password == password {
			return true
		}
	}
	return false
}

func (s *Store) Count() int {
	return len(s.users)
}
