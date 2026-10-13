package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MemoryPath returns the path of the memory file, ~/.logwiz/memory.md.
func MemoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".logwiz", "memory.md"), nil
}

// LoadMemory reads the memory file. A missing file (or an unknown home
// directory) is not an error and yields "".
func LoadMemory() (string, error) {
	path, err := MemoryPath()
	if err != nil {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading ~/.logwiz/memory.md: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// EnsureMemoryFile creates ~/.logwiz/memory.md (and its directory) if it
// does not exist yet, and returns its path.
func EnsureMemoryFile() (string, error) {
	path, err := MemoryPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return "", err
	}
	return path, f.Close()
}

// AppendMemory adds text to the memory file as a single bullet line.
func AppendMemory(text string) error {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return errors.New("nothing to remember")
	}
	if !strings.HasPrefix(text, "- ") && !strings.HasPrefix(text, "* ") {
		text = "- " + text
	}
	path, err := EnsureMemoryFile()
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		text = "\n" + text
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ClearMemory empties the memory file. A missing file is not an error.
func ClearMemory() error {
	path, err := MemoryPath()
	if err != nil {
		return err
	}
	err = os.Truncate(path, 0)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
