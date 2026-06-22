package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed manifests/*.json
var defaultManifestsFS embed.FS

type Architecture struct {
	URL    string   `json:"url"`
	SHA256 string   `json:"sha256"`
	Bin    []string `json:"bin"`
}

type Manifest struct {
	Name          string                  `json:"name"`
	Version       string                  `json:"version"`
	Description   string                  `json:"description"`
	Homepage      string                  `json:"homepage"`
	Architectures map[string]Architecture `json:"architectures"`
}

func (m *Manifest) GetArchitecture(goos, goarch string) (Architecture, bool) {
	key := fmt.Sprintf("%s-%s", goos, goarch)
	arch, ok := m.Architectures[key]
	return arch, ok
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func LoadManifest(name string, homeDir string) (*Manifest, error) {
	name = strings.ToLower(name)
	if !strings.HasSuffix(name, ".json") {
		name = name + ".json"
	}

	localPath := filepath.Join(homeDir, "manifests", name)
	if _, err := os.Stat(localPath); err == nil {
		data, err := os.ReadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read local manifest %s: %w", localPath, err)
		}
		return ParseManifest(data)
	}

	embedPath := "manifests/" + name
	data, err := defaultManifestsFS.ReadFile(embedPath)
	if err == nil {
		return ParseManifest(data)
	}

	onlineURL := fmt.Sprintf("https://raw.githubusercontent.com/akhileshg0111/quest/main/manifests/%s", name)
	resp, err := http.Get(onlineURL)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		onlineData, err := io.ReadAll(resp.Body)
		if err == nil {
			m, err := ParseManifest(onlineData)
			if err == nil {
				_ = SaveLocalManifest(m, homeDir)
				return m, nil
			}
		}
	}

	return nil, fmt.Errorf("manifest not found locally or online: %s", strings.TrimSuffix(name, ".json"))
}

func ListManifests(homeDir string) (map[string]*Manifest, error) {
	manifests := make(map[string]*Manifest)

	entries, err := defaultManifestsFS.ReadDir("manifests")
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, err := defaultManifestsFS.ReadFile("manifests/" + entry.Name())
			if err == nil {
				if m, err := ParseManifest(data); err == nil {
					manifests[strings.ToLower(m.Name)] = m
				}
			}
		}
	}

	localDir := filepath.Join(homeDir, "manifests")
	if localEntries, err := os.ReadDir(localDir); err == nil {
		for _, entry := range localEntries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(localDir, entry.Name()))
			if err == nil {
				if m, err := ParseManifest(data); err == nil {
					manifests[strings.ToLower(m.Name)] = m
				}
			}
		}
	}

	return manifests, nil
}

func SaveLocalManifest(m *Manifest, homeDir string) error {
	localDir := filepath.Join(homeDir, "manifests")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	destPath := filepath.Join(localDir, strings.ToLower(m.Name)+".json")
	return os.WriteFile(destPath, data, 0644)
}
