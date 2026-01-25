package pihole

import (
	"os"

	"github.com/danmuck/edgectl/internal/services"
)

type Pihole struct {
	Runner services.Runner
}

func (Pihole) Name() string {
	return "pihole"
}

func (p Pihole) Status() (any, error) {
	out, err := p.runner().Run("pihole", "status")
	return out, err
}

func (p Pihole) Actions() map[string]services.Action {
	return map[string]services.Action{
		"restart": func() error {
			_, err := p.runner().Run("pihole", "restartdns")
			return err
		},
		"gravity": func() error {
			_, err := p.runner().Run("pihole", "-g")
			return err
		},
		"stream-log": func() error {
			return p.runner().RunStreaming("pihole", []string{"-t"}, os.Stdout, os.Stderr)
		},
	}
}

func (p Pihole) runner() services.Runner {
	if p.Runner != nil {
		return p.Runner
	}
	return services.LocalRunner{}
}
