// Package config loads the settings shared by the server and the command-line tools:
// the repo root, the song data directory, the Gemini API key and config/teachers.json.
package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Teacher struct {
	LanguageCode string `json:"language_code"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
}

type Config struct {
	RepoRoot string
	DataDir  string
	// TTS model, teacher voice per language, and speech style per speed (config/teachers.json).
	TTSModel string
	Teachers map[string]Teacher
	Styles   map[string]string
	// Speeds in the order they appear in teachers.json ("normal", "slow").
	Speeds []string
}

func (c *Config) HasTeacher(language string) bool {
	_, ok := c.Teachers[language]
	return ok
}

// Load finds the repo root (the nearest ancestor of the working directory holding
// config/teachers.json, or $SONG_LINGO_ROOT) and reads the configuration from it.
func Load() (*Config, error) {
	root, err := findRoot()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(root, "config", "teachers.json"))
	if err != nil {
		return nil, err
	}
	var file struct {
		Model    string             `json:"model"`
		Teachers map[string]Teacher `json:"teachers"`
		Styles   map[string]string  `json:"styles"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("config/teachers.json: %w", err)
	}
	speeds, err := objectKeys(raw, "styles")
	if err != nil {
		return nil, fmt.Errorf("config/teachers.json: %w", err)
	}

	dataDir := os.Getenv("SONG_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(root, "output")
	}
	return &Config{
		RepoRoot: root,
		DataDir:  dataDir,
		TTSModel: file.Model,
		Teachers: file.Teachers,
		Styles:   file.Styles,
		Speeds:   speeds,
	}, nil
}

func findRoot() (string, error) {
	if root := os.Getenv("SONG_LINGO_ROOT"); root != "" {
		return filepath.Abs(root)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "config", "teachers.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("找不到 config/teachers.json：請在專案目錄裡執行，或設定 SONG_LINGO_ROOT")
		}
		dir = parent
	}
}

// objectKeys returns the keys of a top-level JSON object field in document order.
func objectKeys(raw []byte, field string) ([]string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(top[field]))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("%q is not an object", field)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

var envLine = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$`)

// APIKey returns GEMINI_API_KEY from the environment or the repo-root .env file.
func (c *Config) APIKey() (string, error) {
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		return key, nil
	}
	if key := readDotEnv(filepath.Join(c.RepoRoot, ".env"))["GEMINI_API_KEY"]; key != "" {
		return key, nil
	}
	return "", errors.New("找不到 GEMINI_API_KEY，請在專案根目錄的 .env 設定。")
}

// readDotEnv accepts the forms python-dotenv does: optional `export`, optional quotes.
func readDotEnv(path string) map[string]string {
	values := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return values
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := envLine.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		value := strings.TrimSpace(m[2])
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		values[m[1]] = value
	}
	return values
}
