package main

// prompt.go contains interactive input utilities (line, multiline, int, and guided selection prompts).

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func (a *App) promptLine(label string) (string, error) {
	if strings.TrimSpace(label) != "" {
		fmt.Printf("%s: ", label)
	}
	line, err := a.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// promptMultiline captures multi-line text until a sentinel line is entered.
func (a *App) promptMultiline(header string, terminator string) (string, error) {
	term := strings.TrimSpace(terminator)
	if term == "" {
		term = ".done"
	}
	fmt.Println(header)
	var lines []string
	for {
		line, err := a.promptLine("")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(line) == term {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) promptInt(label string, min int, max int, allowBack bool, allowExit bool) (int, error) {
	for {
		rangePrompt := fmt.Sprintf("%s [%d-%d", label, min, max)
		if allowBack {
			rangePrompt += "|back|b"
		}
		if allowExit {
			rangePrompt += "|exit|e"
		}
		rangePrompt += "]"
		line, err := a.promptLine(rangePrompt)
		if err != nil {
			return 0, err
		}
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if allowBack && (trimmed == "back" || trimmed == "b") {
			return 0, ErrNavigateBack
		}
		if allowExit && (trimmed == "exit" || trimmed == "e") {
			return 0, ErrNavigateExit
		}
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < min || v > max {
			fmt.Println("Invalid selection.")
			continue
		}
		return v, nil
	}
}

func (a *App) promptOptionalLimit() (int, error) {
	limitRaw, err := a.promptLine("limit (default 20)")
	if err != nil {
		return 0, err
	}
	limit := 20
	if strings.TrimSpace(limitRaw) != "" {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(limitRaw))
		if parseErr != nil || parsed <= 0 {
			return 0, errors.New("limit must be a positive integer")
		}
		limit = parsed
	}
	return limit, nil
}

// promptCommandTemplateSelection renders one guided command template list and returns one selection.
func (a *App) promptCommandTemplateSelection(label string, templates []CommandTemplate) (CommandTemplate, error) {
	fmt.Println("Available Commands")
	for i := range templates {
		tpl := templates[i]
		fmt.Printf("  %d) %s [%s %s]\n", i+1, tpl.Label, tpl.SeedSelector, tpl.Operation)
		if strings.TrimSpace(tpl.Description) != "" {
			fmt.Printf("     - %s\n", tpl.Description)
		}
	}
	choice, err := a.promptInt(label, 1, len(templates), true, true)
	if err != nil {
		return CommandTemplate{}, err
	}
	return templates[choice-1], nil
}

// promptMirageIntentTemplateSelection renders one guided intent list and returns one selection.
func (a *App) promptMirageIntentTemplateSelection(
	label string,
	templates []MirageIntentTemplate,
) (MirageIntentTemplate, error) {
	fmt.Println("Available Intents")
	for i := range templates {
		tpl := templates[i]
		if tpl.Orchestrator != nil {
			deps := strings.Join(tpl.SeedDependencies, "+")
			fmt.Printf("  %d) %s [%s orchestrator]\n", i+1, tpl.Label, deps)
		} else {
			fmt.Printf("  %d) %s [%s %s]\n", i+1, tpl.Label, tpl.Command.SeedSelector, tpl.Command.Operation)
		}
		if strings.TrimSpace(tpl.Description) != "" {
			fmt.Printf("     - %s\n", tpl.Description)
		}
	}
	choice, err := a.promptInt(label, 1, len(templates), true, true)
	if err != nil {
		return MirageIntentTemplate{}, err
	}
	return templates[choice-1], nil
}

// promptGhostIDSelection renders selectable ghost ids for one intent target.
func (a *App) promptGhostIDSelection(label string, ghostIDs []string) (string, error) {
	fmt.Println("Eligible Ghost Targets")
	for i := range ghostIDs {
		fmt.Printf("  %d) %s\n", i+1, ghostIDs[i])
	}
	choice, err := a.promptInt(label, 1, len(ghostIDs), true, true)
	if err != nil {
		return "", err
	}
	return ghostIDs[choice-1], nil
}

// promptCommandArgs collects required/optional argument values for one command template.
func (a *App) promptCommandArgs(specs []CommandArgSpec) (map[string]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(specs))
	for i := range specs {
		spec := specs[i]
		if strings.TrimSpace(spec.Key) == "" {
			continue
		}
		for {
			prompt := strings.TrimSpace(spec.Prompt)
			if prompt == "" {
				prompt = spec.Key
			}
			if strings.TrimSpace(spec.DefaultValue) != "" && !spec.Multiline {
				prompt += fmt.Sprintf(" (default=%s)", spec.DefaultValue)
			}
			value := ""
			if spec.Multiline {
				terminator := strings.TrimSpace(spec.Terminator)
				if terminator == "" {
					terminator = ".done"
				}
				raw, err := a.promptMultiline(
					fmt.Sprintf("%s. Finish with a line containing only %q.", prompt, terminator),
					terminator,
				)
				if err != nil {
					return nil, err
				}
				value = raw
			} else {
				raw, err := a.promptLine(prompt)
				if err != nil {
					return nil, err
				}
				value = strings.TrimSpace(raw)
			}
			if value == "" && strings.TrimSpace(spec.DefaultValue) != "" {
				value = strings.TrimSpace(spec.DefaultValue)
			}
			if spec.Required && value == "" {
				fmt.Printf("Argument %q is required.\n", spec.Key)
				continue
			}
			if spec.Key == "path" && value != "" && filepath.IsAbs(value) {
				fmt.Printf("Argument %q must be a relative path.\n", spec.Key)
				continue
			}
			if value != "" {
				out[spec.Key] = value
			}
			break
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
