package main

import (
	"flag"
	"log"

	"github.com/danmuck/edgectl/internal/config"
)

func main() {
	kind := flag.String("kind", "ghost", "config kind: ghost|seed")
	output := flag.String("output", "", "output path for config template")
	force := flag.Bool("force", false, "overwrite existing config file")
	flag.Parse()

	target := *output
	if target == "" {
		switch *kind {
		case "ghost":
			target = "cmd/edgectl/config.toml"
		case "seed":
			target = "cmd/seedctl/config.toml"
		default:
			log.Fatalf("unknown kind: %s", *kind)
		}
	}

	if err := config.WriteTemplate(target, *kind, *force); err != nil {
		log.Fatal(err)
	}
	log.Printf("Wrote %s config template to %s", *kind, target)
}
