package main

import (
	"flag"
	"log"

	"github.com/danmuck/edgectl/internal/config"
)

func main() {
	kind := flag.String("kind", "mirage", "config kind: mirage|ghost")
	output := flag.String("output", "", "output path for config template")
	validate := flag.Bool("validate", false, "validate an existing config file")
	input := flag.String("input", "", "config path for validation (defaults to per-kind cmd path)")
	force := flag.Bool("force", false, "overwrite existing config file")
	flag.Parse()

	if *validate {
		path := *input
		if path == "" {
			switch *kind {
			case "mirage":
				path = "cmd/miragectl/config.toml"
			case "ghost":
				path = "cmd/ghostctl/config.toml"
			default:
				log.Fatalf("unknown kind: %s", *kind)
			}
		}

		switch *kind {
		case "mirage":
			if _, err := config.LoadMirageConfig(path); err != nil {
				log.Fatal(err)
			}
		case "ghost":
			if _, err := config.LoadGhostConfig(path); err != nil {
				log.Fatal(err)
			}
		default:
			log.Fatalf("unknown kind: %s", *kind)
		}
		log.Printf("Validated %s config at %s", *kind, path)
		return
	}

	target := *output
	if target == "" {
		switch *kind {
		case "mirage":
			target = "cmd/miragectl/config.toml"
		case "ghost":
			target = "cmd/ghostctl/config.toml"
		default:
			log.Fatalf("unknown kind: %s", *kind)
		}
	}

	if err := config.WriteTemplate(target, *kind, *force); err != nil {
		log.Fatal(err)
	}
	log.Printf("Wrote %s config template to %s", *kind, target)
}
