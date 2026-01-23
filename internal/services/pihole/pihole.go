package pihole

import (
	"os/exec"

	"github.com/danmuck/edgectl/internal/plugins"
)

type Plugin struct{}

func (Plugin) Name() string {
	return "pihole"
}

func (Plugin) Status() (any, error) {
	out, err := exec.Command("pihole", "status").Output()
	return string(out), err
}

func (Plugin) Actions() map[string]plugins.Action {
	return map[string]plugins.Action{
		"restart": func() error {
			return exec.Command("pihole", "restartdns").Run()
		},
		"gravity": func() error {
			return exec.Command("pihole", "-g").Run()
		},
	}
}
