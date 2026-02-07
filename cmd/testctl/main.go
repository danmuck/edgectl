package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type options struct {
	mode string
	pkg  string
	run  string
}

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

type packageTests struct {
	ImportPath string
	RelPath    string
	Group      string
	Tests      []string
}

type packageStats struct {
	testsRun  int
	testsPass int
	testsFail int
	testsSkip int
	status    string
	elapsed   float64
}

type runSummary struct {
	packagesTotal int
	packagesPass  int
	packagesFail  int
	testsRun      int
	testsPass     int
	testsFail     int
	testsSkip     int
	failures      []string
}

var (
	listNamePattern    = regexp.MustCompile(`^(Test|Benchmark|Fuzz|Example)[A-Za-z0-9_]+$`)
	boundaryLinePrefix = regexp.MustCompile(`^(=== RUN|=== PAUSE|=== CONT|--- PASS:|--- FAIL:|--- SKIP:)`)
	packageLinePrefix  = regexp.MustCompile(`^(ok|FAIL|\?)\s+`)
)

func main() {
	opts := parseFlags()
	switch opts.mode {
	case "list":
		if err := runList(opts); err != nil {
			fatalf("%v", err)
		}
	case "run":
		exitCode, err := runTests(opts)
		if err != nil {
			fatalf("%v", err)
		}
		os.Exit(exitCode)
	default:
		fatalf("unknown mode %q (supported: run, list)", opts.mode)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.mode, "mode", "run", "mode: run | list")
	flag.StringVar(&opts.pkg, "pkg", "./...", "package pattern(s), comma-separated or space-separated")
	flag.StringVar(&opts.run, "run", "", "go test -run regex (run mode)")
	flag.Parse()
	return opts
}

func runList(opts options) error {
	modulePath, err := goListModulePath()
	if err != nil {
		return err
	}
	patterns := parsePatterns(opts.pkg)
	packages, err := goListPackages(patterns)
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		fmt.Println("No packages matched.")
		return nil
	}

	byGroup := make(map[string][]packageTests)
	for _, pkg := range packages {
		tests, err := listTestsForPackage(pkg)
		if err != nil {
			return err
		}
		rel := relImportPath(modulePath, pkg)
		group := moduleGroup(rel)
		byGroup[group] = append(byGroup[group], packageTests{
			ImportPath: pkg,
			RelPath:    rel,
			Group:      group,
			Tests:      tests,
		})
	}

	groups := sortedKeys(byGroup)
	totalPackages := 0
	totalTests := 0

	fmt.Println("Test Inventory")
	fmt.Printf("Patterns: %s\n", strings.Join(patterns, ", "))
	fmt.Println()

	for _, group := range groups {
		pkgList := byGroup[group]
		sort.Slice(pkgList, func(i, j int) bool {
			return pkgList[i].RelPath < pkgList[j].RelPath
		})
		groupTests := 0
		for _, p := range pkgList {
			groupTests += len(p.Tests)
		}
		totalPackages += len(pkgList)
		totalTests += groupTests

		fmt.Printf("Module: %s  (packages=%d tests=%d)\n", group, len(pkgList), groupTests)
		for _, p := range pkgList {
			fmt.Printf("  Package: %s", p.RelPath)
			if len(p.Tests) == 0 {
				fmt.Println("  [no tests]")
				continue
			}
			fmt.Printf("  [tests=%d]\n", len(p.Tests))
			for _, testName := range p.Tests {
				fmt.Printf("    - %s\n", testName)
			}
		}
		fmt.Println()
	}

	fmt.Println("Summary")
	fmt.Printf("  Modules:  %d\n", len(groups))
	fmt.Printf("  Packages: %d\n", totalPackages)
	fmt.Printf("  Tests:    %d\n", totalTests)
	return nil
}

func runTests(opts options) (int, error) {
	latestRunSummary = runSummary{}

	modulePath, err := goListModulePath()
	if err != nil {
		return 1, err
	}
	patterns := parsePatterns(opts.pkg)
	args := []string{"test", "-json", "-p", "1"}
	if strings.TrimSpace(opts.run) != "" {
		args = append(args, "-run", opts.run)
	}
	args = append(args, patterns...)

	cmd := exec.Command("go", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 1, err
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return 1, err
	}

	errc := make(chan error, 2)
	go func() {
		errc <- streamTestEvents(modulePath, stdout)
	}()
	go func() {
		errc <- streamStderr(stderr)
	}()

	waitErr := cmd.Wait()
	streamErrA := <-errc
	streamErrB := <-errc
	if streamErrA != nil {
		return 1, streamErrA
	}
	if streamErrB != nil {
		return 1, streamErrB
	}

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return 1, waitErr
		}
	}

	printRunSummary(start)
	return exitCode, nil
}

var latestRunSummary runSummary

func streamTestEvents(modulePath string, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	stats := make(map[string]*packageStats)
	seenPackage := make(map[string]bool)
	packageOrder := make([]string, 0)
	failures := make([]string, 0)
	currentPackage := ""

	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev testEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			fmt.Printf("raw> %s\n", strings.TrimSpace(string(line)))
			continue
		}

		pkg := ev.Package
		if pkg != "" && !seenPackage[pkg] {
			seenPackage[pkg] = true
			packageOrder = append(packageOrder, pkg)
		}
		if pkg != "" && pkg != currentPackage {
			currentPackage = pkg
			rel := relImportPath(modulePath, pkg)
			fmt.Printf("\nPackage: %s\n", rel)
		}
		if pkg != "" {
			if _, ok := stats[pkg]; !ok {
				stats[pkg] = &packageStats{}
			}
		}

		switch ev.Action {
		case "run":
			if ev.Test != "" {
				stats[pkg].testsRun++
				fmt.Printf("  [RUN ] %s\n", ev.Test)
			}
		case "pass":
			if ev.Test != "" {
				stats[pkg].testsPass++
				fmt.Printf("  [PASS] %s (%.2fs)\n", ev.Test, ev.Elapsed)
				break
			}
			stats[pkg].status = "pass"
			stats[pkg].elapsed = ev.Elapsed
			fmt.Printf("[PASS] package (%.2fs)\n", ev.Elapsed)
		case "fail":
			if ev.Test != "" {
				stats[pkg].testsFail++
				rel := relImportPath(modulePath, pkg)
				failures = append(failures, fmt.Sprintf("%s:%s", rel, ev.Test))
				fmt.Printf("  [FAIL] %s (%.2fs)\n", ev.Test, ev.Elapsed)
				break
			}
			stats[pkg].status = "fail"
			stats[pkg].elapsed = ev.Elapsed
			fmt.Printf("[FAIL] package (%.2fs)\n", ev.Elapsed)
		case "skip":
			if ev.Test != "" {
				stats[pkg].testsSkip++
				fmt.Printf("  [SKIP] %s (%.2fs)\n", ev.Test, ev.Elapsed)
				break
			}
			if stats[pkg].status == "" {
				stats[pkg].status = "skip"
				stats[pkg].elapsed = ev.Elapsed
				fmt.Printf("[SKIP] package (%.2fs)\n", ev.Elapsed)
			}
		case "output":
			renderOutputLine(ev.Output, ev.Test != "")
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	summary := runSummary{}
	summary.packagesTotal = len(packageOrder)
	for _, pkg := range packageOrder {
		ps := stats[pkg]
		summary.testsRun += ps.testsRun
		summary.testsPass += ps.testsPass
		summary.testsFail += ps.testsFail
		summary.testsSkip += ps.testsSkip
		if ps.status == "fail" {
			summary.packagesFail++
		} else {
			summary.packagesPass++
		}
	}
	summary.failures = failures
	latestRunSummary = summary
	return nil
}

func streamStderr(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 16*1024), 2*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fmt.Printf("stderr> %s\n", line)
	}
	return sc.Err()
}

func renderOutputLine(raw string, withinTest bool) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return
	}
	if boundaryLinePrefix.MatchString(line) {
		return
	}
	if line == "PASS" || line == "FAIL" {
		return
	}
	if packageLinePrefix.MatchString(line) {
		return
	}
	prefix := "  |"
	if withinTest {
		prefix = "    |"
	}
	fmt.Printf("%s %s\n", prefix, line)
}

func printRunSummary(start time.Time) {
	totalDuration := time.Since(start)
	fmt.Println()
	fmt.Println("Summary")
	fmt.Printf("  Packages: total=%d pass=%d fail=%d\n",
		latestRunSummary.packagesTotal,
		latestRunSummary.packagesPass,
		latestRunSummary.packagesFail,
	)
	fmt.Printf("  Tests:    run=%d pass=%d fail=%d skip=%d\n",
		latestRunSummary.testsRun,
		latestRunSummary.testsPass,
		latestRunSummary.testsFail,
		latestRunSummary.testsSkip,
	)
	fmt.Printf("  Duration: %s\n", totalDuration.Round(time.Millisecond))
	if len(latestRunSummary.failures) > 0 {
		fmt.Println("  Failed Tests:")
		for _, name := range latestRunSummary.failures {
			fmt.Printf("    - %s\n", name)
		}
	}
}

func parsePatterns(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"./..."}
	}
	chunks := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return []string{"./..."}
	}
	return out
}

func goListModulePath() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Path}}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func goListPackages(patterns []string) ([]string, error) {
	args := append([]string{"list"}, patterns...)
	out, err := exec.Command("go", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	outPkgs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		outPkgs = append(outPkgs, line)
	}
	sort.Strings(outPkgs)
	return outPkgs, nil
}

func listTestsForPackage(pkg string) ([]string, error) {
	out, err := exec.Command("go", "test", pkg, "-list", ".").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go test -list failed for %s: %w: %s", pkg, err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(string(out), "\n")
	tests := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !listNamePattern.MatchString(line) {
			continue
		}
		tests = append(tests, line)
	}
	sort.Strings(tests)
	return tests, nil
}

func relImportPath(modulePath string, importPath string) string {
	if importPath == modulePath {
		return "."
	}
	prefix := modulePath + "/"
	if strings.HasPrefix(importPath, prefix) {
		return strings.TrimPrefix(importPath, prefix)
	}
	return importPath
}

func moduleGroup(relPath string) string {
	if relPath == "." {
		return "root"
	}
	parts := strings.Split(relPath, "/")
	if len(parts) == 0 {
		return "misc"
	}
	if parts[0] == "cmd" {
		return "cmd"
	}
	if parts[0] == "internal" {
		if len(parts) >= 2 && parts[1] == "protocol" {
			return "internal/protocol"
		}
		if len(parts) >= 2 {
			return "internal/" + parts[1]
		}
		return "internal"
	}
	return parts[0]
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "testctl: "+format+"\n", args...)
	os.Exit(1)
}
