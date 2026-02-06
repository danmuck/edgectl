package pihole

import (
	"os"

	"github.com/danmuck/edgectl/internal/seeds"
)

type Pihole struct {
	Runner seeds.Runner
}

func (Pihole) Name() string {
	return "pihole"
}

func (p Pihole) Status() (any, error) {
	out, err := p.runner().Run("pihole", "status")
	return out, err
}

func (p Pihole) Actions() map[string]seeds.Action {
	return map[string]seeds.Action{
		"restart": func() (string, error) {
			return p.runner().Run("pihole", "restartdns")
		},
		"gravity": func() (string, error) {
			return p.runner().Run("pihole", "-g")
		},
		"stream-log": func() (string, error) {
			err := p.runner().RunStreaming("pihole", []string{"-t"}, os.Stdout, os.Stderr)
			return "", err
		},
	}
}

func (p Pihole) runner() seeds.Runner {
	if p.Runner != nil {
		return p.Runner
	}
	return seeds.LocalRunner{}
}
