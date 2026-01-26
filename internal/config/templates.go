package config

import (
	"fmt"
	"os"
	"strings"
)

func Template(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "ghost":
		return ghostTemplate, nil
	case "seed":
		return seedTemplate, nil
	default:
		return "", fmt.Errorf("unknown config kind: %s", kind)
	}
}

func WriteTemplate(path, kind string, overwrite bool) error {
	template, err := Template(kind)
	if err != nil {
		return err
	}
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists: %s", path)
		}
	}
	return os.WriteFile(path, []byte(template), 0o600)
}

const ghostTemplate = `name = "edge-ctl"
addr = ":9000"
cors_origins = ["http://localhost:3000"]

[[seeds]]
id = "edge-ctl"
host = "localhost"
addr = "localhost:9000"
group = "root"
exec = true
auth = "temp-auth-key"

[[seeds]]
id = "infra"
host = "localhost"
addr = "localhost:8080"
group = "root"
exec = true
auth = "temp-auth-infra-key"
`

const seedTemplate = `id = "seedctl"
addr = ":9100"
host = "raspberrypi"
group = "root"
exec = true
cors_origins = ["http://localhost:3000"]
services = []
`
