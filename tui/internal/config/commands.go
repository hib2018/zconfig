package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

type fileConfig struct {
	Agents     []registrationJSON `json:"agents"`
	Validators []registrationJSON `json:"validators"`
}
type registrationJSON struct {
	Name                 string   `json:"name"`
	Executable           string   `json:"executable"`
	Args                 []string `json:"args"`
	WorkingDirectory     string   `json:"working_directory,omitempty"`
	EnvironmentAllowlist []string `json:"environment_allowlist,omitempty"`
	TimeoutSeconds       int      `json:"timeout_seconds,omitempty"`
	ProtocolMajor        int      `json:"protocol_major,omitempty"`
}
type Config struct {
	Agents     []Registration
	Validators []Registration
}
type Registration struct {
	Name, Executable, WorkingDirectory string
	Args, EnvironmentAllowlist         []string
	Timeout                            time.Duration
	ProtocolMajor                      int
}

func Load(path, projectDirectory string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var raw fileConfig
	if err := protocol.DecodePayload(data, &raw); err != nil {
		return Config{}, err
	}
	convert := func(values []registrationJSON, defaultTimeout time.Duration) ([]Registration, error) {
		out := make([]Registration, 0, len(values))
		names := map[string]bool{}
		for _, v := range values {
			if v.Name == "" || names[v.Name] {
				return nil, errors.New("registration names must be non-empty and unique")
			}
			names[v.Name] = true
			if strings.TrimSpace(v.Executable) != v.Executable || v.Executable == "" || strings.ContainsAny(v.Executable, "\t\r\n") || strings.Contains(v.Executable, " ") {
				return nil, errors.New("executable must be one literal program name or path")
			}
			for _, a := range v.Args {
				if strings.Contains(a, "$HOME") || strings.Contains(a, "${") || strings.ContainsAny(a, "\r\n") {
					return nil, errors.New("arguments may not contain expansion syntax")
				}
			}
			wd := v.WorkingDirectory
			if wd == "" {
				wd = projectDirectory
			}
			if !filepath.IsAbs(wd) {
				wd = filepath.Join(projectDirectory, wd)
			}
			info, err := os.Stat(wd)
			if err != nil || !info.IsDir() {
				return nil, fmt.Errorf("working directory %q is invalid", wd)
			}
			timeout := defaultTimeout
			if v.TimeoutSeconds < 0 {
				return nil, errors.New("timeout must be positive")
			}
			if v.TimeoutSeconds > 0 {
				timeout = time.Duration(v.TimeoutSeconds) * time.Second
			}
			major := v.ProtocolMajor
			if major == 0 {
				major = 1
			}
			if major != 1 {
				return nil, errors.New("unsupported protocol major")
			}
			out = append(out, Registration{Name: v.Name, Executable: v.Executable, Args: append([]string(nil), v.Args...), WorkingDirectory: wd, EnvironmentAllowlist: append([]string(nil), v.EnvironmentAllowlist...), Timeout: timeout, ProtocolMajor: major})
		}
		return out, nil
	}
	agents, err := convert(raw.Agents, 120*time.Second)
	if err != nil {
		return Config{}, err
	}
	validators, err := convert(raw.Validators, 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	return Config{Agents: agents, Validators: validators}, nil
}
func (r Registration) Environment() []string {
	out := make([]string, 0, len(r.EnvironmentAllowlist))
	for _, name := range r.EnvironmentAllowlist {
		if name == "" || strings.Contains(name, "=") {
			continue
		}
		if value, ok := os.LookupEnv(name); ok {
			out = append(out, name+"="+value)
		}
	}
	return out
}
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "zconfig", "commands.json"), nil
}
