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
	"strconv"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
)

// testctl CLI flags for execution mode and selection scope.
type options struct {
	mode string
	pkg  string
	run  string
}

// testctl pacing mode for interactive execution.
type pacingMode string

const (
	pacingFree  pacingMode = "free"
	pacingPause pacingMode = "pause"
)

// testctl runtime options for one test execution pass.
type runConfig struct {
	pacing     pacingMode
	pauseDelay time.Duration
	pauseInput *bufio.Reader
}

// testctl one package/test selection used by pause-mode execution.
type testCase struct {
	ImportPath string
	RelPath    string
	Name       string
}

// testctl mirror of `go test -json` event fields consumed by renderer.
type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
	Output  string  `json:"Output"`
}

// testctl discovered test names for one package.
type packageTests struct {
	ImportPath string
	RelPath    string
	Group      string
	Tests      []string
}

// testctl grouped package/test inventory by module bucket.
type inventory struct {
	modulePath string
	groups     []string
	byGroup    map[string][]packageTests
	packages   []packageTests
}

// testctl per-test outcome row used in summary rendering.
type testResult struct {
	Name    string
	Status  string
	Elapsed float64
}

// testctl per-package outcomes plus per-test result rows.
type packageResult struct {
	ImportPath string
	RelPath    string
	Status     string
	Elapsed    float64
	Tests      []testResult
}

// testctl live counters while streaming test events.
type packageStats struct {
	testsRun  int
	testsPass int
	testsFail int
	testsSkip int
	status    string
	elapsed   float64
}

// testctl package collector keeping output order stable while updating test rows.
type packageCollector struct {
	result    packageResult
	testIndex map[string]int
}

// testctl aggregate report printed after each run.
type runSummary struct {
	packagesTotal  int
	packagesPass   int
	packagesFail   int
	testsRun       int
	testsPass      int
	testsFail      int
	testsSkip      int
	failures       []string
	packageResults []packageResult
}

// testctl terminal progress indicator for background pre-run discovery steps.
type progressBar struct {
	label string
	total int
	done  int
	// lastLineWidth tracks previous rendered width so shorter updates can clear remnants.
	lastLineWidth int
}

var (
	listNamePattern    = regexp.MustCompile(`^(Test|Benchmark|Fuzz|Example)[A-Za-z0-9_]+$`)
	boundaryLinePrefix = regexp.MustCompile(`^(=== RUN|=== PAUSE|=== CONT|--- PASS:|--- FAIL:|--- SKIP:)`)
	packageLinePrefix  = regexp.MustCompile(`^(ok|FAIL|\?)\s+`)
	ansiEscapePattern  = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	latestRunSummary   runSummary
)

const (
	ansiGreen   = "\x1b[32m"
	ansiMagenta = "\x1b[35m"
	ansiReset   = "\x1b[0m"
)

var fieldNameByID = map[uint16]string{
	schema.FieldIntentID:             "intent_id",
	schema.FieldCommandID:            "command_id",
	schema.FieldExecutionID:          "execution_id",
	schema.FieldEventID:              "event_id",
	schema.FieldPhase:                "phase",
	schema.FieldTimestampMS:          "timestamp_ms",
	schema.FieldActor:                "actor",
	schema.FieldTargetScope:          "target_scope",
	schema.FieldObjective:            "objective",
	schema.FieldGhostID:              "ghost_id",
	schema.FieldSeedSelector:         "seed_selector",
	schema.FieldOperation:            "operation",
	schema.FieldArgs:                 "args",
	schema.FieldSeedID:               "seed_id",
	schema.FieldSeedExecuteOperation: "seed_execute_operation",
	schema.FieldSeedExecuteArgs:      "seed_execute_args",
	schema.FieldStatus:               "status",
	schema.FieldStdout:               "stdout",
	schema.FieldStderr:               "stderr",
	schema.FieldExitCode:             "exit_code",
	schema.FieldOutcome:              "outcome",
	schema.FieldSummary:              "summary",
	schema.FieldCompletionState:      "completion_state",
	schema.FieldAckStatus:            "ack_status",
	schema.FieldAckCode:              "ack_code",
}

var tlvTypeNameByID = map[uint8]string{
	tlv.TypeU8:     "u8",
	tlv.TypeU16:    "u16",
	tlv.TypeU32:    "u32",
	tlv.TypeU64:    "u64",
	tlv.TypeBool:   "bool",
	tlv.TypeString: "string",
	tlv.TypeBytes:  "bytes",
}

var messageTypeNameByID = map[uint32]string{
	schema.MsgIssue:       "issue",
	schema.MsgCommand:     "command",
	schema.MsgSeedExecute: "seed.execute",
	schema.MsgSeedResult:  "seed.result",
	schema.MsgEvent:       "event",
	schema.MsgReport:      "report",
	schema.MsgError:       "error",
	schema.MsgEventAck:    "event.ack",
}

// testctl mode dispatcher for list/run/interactive.
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
	case "interactive":
		exitCode, err := runInteractive(opts)
		if err != nil {
			fatalf("%v", err)
		}
		os.Exit(exitCode)
	default:
		fatalf("unknown mode %q (supported: run, list, interactive)", opts.mode)
	}
}

// testctl flag parser into options.
func parseFlags() options {
	var opts options
	flag.StringVar(&opts.mode, "mode", "run", "mode: run | list | interactive")
	flag.StringVar(&opts.pkg, "pkg", "./...", "package pattern(s), comma-separated or space-separated")
	flag.StringVar(&opts.run, "run", "", "go test -run regex (run mode)")
	flag.Parse()
	return opts
}

// testctl interactive selector UI for choosing test scope.
func runInteractive(opts options) (int, error) {
	reader := bufio.NewReader(os.Stdin)
	cfg := defaultRunConfig()
	cachedInventory := (*inventory)(nil)
	loadInventory := func() (*inventory, error) {
		if cachedInventory != nil {
			return cachedInventory, nil
		}
		inv, err := buildInventory(parsePatterns(opts.pkg))
		if err != nil {
			return nil, err
		}
		cachedInventory = &inv
		return cachedInventory, nil
	}

	for {
		fmt.Println("Interactive Test Runner")
		fmt.Println("  1) Run all tests")
		fmt.Println("  2) Select module")
		fmt.Println("  3) Select package")
		fmt.Printf("  4) Toggle pacing (%s)\n", cfg.pacing)
		fmt.Println("  q) Exit")

		choice, err := promptInt(reader, "Choose an option", 1, 4)
		if err != nil {
			if errors.Is(err, errPromptBack) {
				fmt.Println("Exiting.")
				return 0, nil
			}
			return 1, err
		}

		switch choice {
		case 1:
			cfg.pauseInput = reader
			if _, err := runTestsWithConfig(options{mode: "run", pkg: "./...", run: opts.run}, cfg); err != nil {
				fmt.Printf("Run error: %v\n", err)
			}
			fmt.Println()
		case 2:
			inv, err := loadInventory()
			if err != nil {
				return 1, err
			}
			if len(inv.groups) == 0 {
				fmt.Println("No modules available.")
				fmt.Println()
				continue
			}
			if err := runModuleMenu(reader, opts, inv, &cfg); err != nil {
				return 1, err
			}
		case 3:
			inv, err := loadInventory()
			if err != nil {
				return 1, err
			}
			if len(inv.packages) == 0 {
				fmt.Println("No packages matched.")
				fmt.Println()
				continue
			}
			if err := runPackageMenu(reader, opts, inv, &cfg); err != nil {
				return 1, err
			}
		case 4:
			if cfg.pacing == pacingFree {
				cfg.pacing = pacingPause
			} else {
				cfg.pacing = pacingFree
			}
			fmt.Printf("Pacing set to %s.\n\n", cfg.pacing)
		}
	}
}

func runModuleMenu(reader *bufio.Reader, opts options, inv *inventory, cfg *runConfig) error {
	for {
		fmt.Println("Modules")
		for i, g := range inv.groups {
			pkgCount := len(inv.byGroup[g])
			testCount := 0
			for _, p := range inv.byGroup[g] {
				testCount += len(p.Tests)
			}
			fmt.Printf("  %d) %s (packages=%d tests=%d)\n", i+1, g, pkgCount, testCount)
		}
		fmt.Println("  q) Back")

		idx, err := promptInt(reader, "Select module", 1, len(inv.groups))
		if err != nil {
			if errors.Is(err, errPromptBack) {
				fmt.Println()
				return nil
			}
			return err
		}

		group := inv.groups[idx-1]
		pkgs := make([]string, 0, len(inv.byGroup[group]))
		for _, p := range inv.byGroup[group] {
			pkgs = append(pkgs, p.ImportPath)
		}
		cfg.pauseInput = reader
		if _, err := runTestsWithConfig(options{mode: "run", pkg: strings.Join(pkgs, ","), run: opts.run}, *cfg); err != nil {
			fmt.Printf("Run error: %v\n", err)
		}
		clearTerminal(os.Stdout)
		fmt.Println()
	}
}

func runPackageMenu(reader *bufio.Reader, opts options, inv *inventory, cfg *runConfig) error {
	for {
		fmt.Println("Packages")
		for i, p := range inv.packages {
			fmt.Printf("  %d) %s  (module=%s tests=%d)\n", i+1, p.RelPath, p.Group, len(p.Tests))
		}
		fmt.Println("  q) Back")

		idx, err := promptInt(reader, "Select package", 1, len(inv.packages))
		if err != nil {
			if errors.Is(err, errPromptBack) {
				fmt.Println()
				return nil
			}
			return err
		}

		pkg := inv.packages[idx-1].ImportPath
		cfg.pauseInput = reader
		if _, err := runTestsWithConfig(options{mode: "run", pkg: pkg, run: opts.run}, *cfg); err != nil {
			fmt.Printf("Run error: %v\n", err)
		}
		fmt.Println()
	}
}

// testctl bounded integer prompt reader from stdin.
func promptInt(reader *bufio.Reader, label string, min int, max int) (int, error) {
	for {
		fmt.Printf("%s [%d-%d, q=back]: ", label, min, max)
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, fmt.Errorf("input closed")
			}
			return 0, err
		}
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "q") {
			return 0, errPromptBack
		}
		v, err := strconv.Atoi(line)
		if err != nil || v < min || v > max {
			fmt.Println("Invalid selection.")
			continue
		}
		return v, nil
	}
}

// testctl inventory printer grouped by module bucket.
func runList(opts options) error {
	inv, err := buildInventory(parsePatterns(opts.pkg))
	if err != nil {
		return err
	}
	if len(inv.packages) == 0 {
		fmt.Println("No packages matched.")
		return nil
	}

	totalPackages := 0
	totalTests := 0

	fmt.Println("Test Inventory")
	fmt.Printf("Patterns: %s\n", strings.Join(parsePatterns(opts.pkg), ", "))
	fmt.Println()

	for _, group := range inv.groups {
		pkgList := inv.byGroup[group]
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
	fmt.Printf("  Modules:  %d\n", len(inv.groups))
	fmt.Printf("  Packages: %d\n", totalPackages)
	fmt.Printf("  Tests:    %d\n", totalTests)
	return nil
}

// testctl inventory builder from package patterns and discovered tests.
func buildInventory(patterns []string) (inventory, error) {
	modulePath, err := goListModulePath()
	if err != nil {
		return inventory{}, err
	}
	packages, err := goListPackages(patterns)
	if err != nil {
		return inventory{}, err
	}
	byGroup := make(map[string][]packageTests)
	all := make([]packageTests, 0, len(packages))
	pb := newProgressBar("Building test inventory", len(packages))
	defer pb.finish()

	for _, pkg := range packages {
		tests, err := listTestsForPackage(pkg)
		if err != nil {
			return inventory{}, err
		}
		rel := relImportPath(modulePath, pkg)
		group := moduleGroup(rel)
		pt := packageTests{
			ImportPath: pkg,
			RelPath:    rel,
			Group:      group,
			Tests:      tests,
		}
		byGroup[group] = append(byGroup[group], pt)
		all = append(all, pt)
		pb.tick(rel)
	}

	groups := sortedKeys(byGroup)
	for g := range byGroup {
		sort.Slice(byGroup[g], func(i, j int) bool {
			return byGroup[g][i].RelPath < byGroup[g][j].RelPath
		})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].RelPath < all[j].RelPath
	})

	return inventory{
		modulePath: modulePath,
		groups:     groups,
		byGroup:    byGroup,
		packages:   all,
	}, nil
}

// testctl execution path for `go test -json` with streamed summaries.
func runTests(opts options) (int, error) {
	return runTestsWithConfig(opts, defaultRunConfig())
}

func defaultRunConfig() runConfig {
	return runConfig{
		pacing:     pacingPause,
		pauseDelay: 150 * time.Millisecond,
	}
}

// runTestsWithConfig executes tests in free or pause pacing mode.
func runTestsWithConfig(opts options, cfg runConfig) (int, error) {
	modulePath, err := goListModulePath()
	if err != nil {
		return 1, err
	}
	patterns := parsePatterns(opts.pkg)
	if cfg.pacing == pacingPause {
		return runTestsPaused(modulePath, patterns, opts.run, cfg)
	}
	args := buildGoTestJSONArgs(patterns, opts.run)
	return runGoTestJSON(modulePath, args)
}

func buildGoTestJSONArgs(patterns []string, runExpr string) []string {
	args := []string{"test", "-json", "-p", "1"}
	if strings.TrimSpace(runExpr) != "" {
		args = append(args, "-run", runExpr)
	}
	args = append(args, patterns...)
	return args
}

func runTestsPaused(modulePath string, patterns []string, runExpr string, cfg runConfig) (int, error) {
	fmt.Println("Pre-run discovery in progress...")
	cases, err := collectTestCases(modulePath, patterns, runExpr)
	if err != nil {
		return 1, err
	}
	if len(cases) == 0 {
		fmt.Println("No tests matched for paused run.")
		return 0, nil
	}
	reader := cfg.pauseInput
	if reader == nil {
		reader = bufio.NewReader(os.Stdin)
	}
	exitCode := 0
	for i, tc := range cases {
		if i > 0 {
			if cfg.pauseDelay > 0 {
				time.Sleep(cfg.pauseDelay)
			}
			if err := waitForNextTest(reader, i+1, len(cases), tc); err != nil {
				if errors.Is(err, errPausedRunAborted) {
					fmt.Println("Paused run aborted by user.")
					return exitCode, nil
				}
				return 1, err
			}
		}
		clearTerminal(os.Stdout)
		fmt.Println()
		fmt.Printf("Step %d/%d  package=%s  test=%s\n", i+1, len(cases), tc.RelPath, tc.Name)
		args := []string{
			"test", "-json", "-p", "1",
			tc.ImportPath,
			"-run", "^" + regexp.QuoteMeta(tc.Name) + "$",
		}
		stepCode, err := runGoTestJSON(modulePath, args)
		if err != nil {
			return 1, err
		}
		if stepCode != 0 {
			exitCode = stepCode
		}
	}
	return exitCode, nil
}

var errPausedRunAborted = errors.New("paused run aborted")
var errPromptBack = errors.New("prompt back requested")

func clearTerminal(w io.Writer) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprint(w, "\033[2J\033[H")
}

func waitForNextTest(reader *bufio.Reader, idx int, total int, next testCase) error {
	fmt.Printf(
		"Paused [%d/%d]. Next: %s:%s  (ENTER=continue, q=stop): ",
		idx,
		total,
		next.RelPath,
		next.Name,
	)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errPausedRunAborted
		}
		return err
	}
	if strings.EqualFold(strings.TrimSpace(line), "q") {
		return errPausedRunAborted
	}
	return nil
}

func collectTestCases(modulePath string, patterns []string, runExpr string) ([]testCase, error) {
	packages, err := goListPackages(patterns)
	if err != nil {
		return nil, err
	}
	var runFilter *regexp.Regexp
	if strings.TrimSpace(runExpr) != "" {
		runFilter, err = regexp.Compile(runExpr)
		if err != nil {
			return nil, fmt.Errorf("invalid -run regex %q: %w", runExpr, err)
		}
	}

	out := make([]testCase, 0)
	pb := newProgressBar("Discovering tests", len(packages))
	defer pb.finish()
	for _, pkg := range packages {
		tests, err := listTestsForPackage(pkg)
		if err != nil {
			return nil, err
		}
		rel := relImportPath(modulePath, pkg)
		for _, testName := range tests {
			if runFilter != nil && !runFilter.MatchString(testName) {
				continue
			}
			out = append(out, testCase{
				ImportPath: pkg,
				RelPath:    rel,
				Name:       testName,
			})
		}
		pb.tick(rel)
	}
	return out, nil
}

func newProgressBar(label string, total int) *progressBar {
	pb := &progressBar{
		label: strings.TrimSpace(label),
		total: total,
	}
	pb.render()
	return pb
}

func (p *progressBar) tick(item string) {
	p.done++
	p.renderWithItem(item)
}

func (p *progressBar) finish() {
	if p.done < p.total {
		p.done = p.total
	}
	p.render()
	fmt.Println()
}

func (p *progressBar) render() {
	p.renderWithItem("")
}

func (p *progressBar) renderWithItem(item string) {
	const width = 28
	total := p.total
	if total <= 0 {
		total = 1
	}
	done := p.done
	if done < 0 {
		done = 0
	}
	if done > total {
		done = total
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	pct := done * 100 / total
	line := fmt.Sprintf("\r%s [%s] %3d%% (%d/%d)", p.label, bar, pct, done, p.total)
	if trimmed := strings.TrimSpace(item); trimmed != "" {
		line += "  " + trimmed
	}
	pad := ""
	currentWidth := len([]rune(strings.TrimPrefix(line, "\r")))
	if currentWidth < p.lastLineWidth {
		pad = strings.Repeat(" ", p.lastLineWidth-currentWidth)
	}
	fmt.Print(line + pad)
	if currentWidth > p.lastLineWidth {
		p.lastLineWidth = currentWidth
	}
}

func runGoTestJSON(modulePath string, args []string) (int, error) {
	latestRunSummary = runSummary{}

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

// testctl JSON-event stream consumer building summary state.
func streamTestEvents(modulePath string, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	stats := make(map[string]*packageStats)
	collectors := make(map[string]*packageCollector)
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
			if _, ok := collectors[pkg]; !ok {
				collectors[pkg] = &packageCollector{
					result: packageResult{
						ImportPath: pkg,
						RelPath:    relImportPath(modulePath, pkg),
					},
					testIndex: make(map[string]int),
				}
			}
		}

		switch ev.Action {
		case "run":
			if ev.Test != "" {
				stats[pkg].testsRun++
				tr := ensureTestResult(collectors[pkg], ev.Test)
				if tr.Status == "" {
					tr.Status = "RUN"
				}
				fmt.Printf("  [RUN ] %s\n", ev.Test)
			}
		case "pass":
			if ev.Test != "" {
				stats[pkg].testsPass++
				tr := ensureTestResult(collectors[pkg], ev.Test)
				tr.Status = "PASS"
				tr.Elapsed = ev.Elapsed
				fmt.Printf("  [PASS] %s (%.2fs)\n", ev.Test, ev.Elapsed)
				break
			}
			stats[pkg].status = "pass"
			stats[pkg].elapsed = ev.Elapsed
			collectors[pkg].result.Status = "PASS"
			collectors[pkg].result.Elapsed = ev.Elapsed
			fmt.Printf("[PASS] package (%.2fs)\n", ev.Elapsed)
		case "fail":
			if ev.Test != "" {
				stats[pkg].testsFail++
				tr := ensureTestResult(collectors[pkg], ev.Test)
				tr.Status = "FAIL"
				tr.Elapsed = ev.Elapsed
				rel := relImportPath(modulePath, pkg)
				failures = append(failures, fmt.Sprintf("%s:%s", rel, ev.Test))
				fmt.Printf("  [FAIL] %s (%.2fs)\n", ev.Test, ev.Elapsed)
				break
			}
			stats[pkg].status = "fail"
			stats[pkg].elapsed = ev.Elapsed
			collectors[pkg].result.Status = "FAIL"
			collectors[pkg].result.Elapsed = ev.Elapsed
			fmt.Printf("[FAIL] package (%.2fs)\n", ev.Elapsed)
		case "skip":
			if ev.Test != "" {
				stats[pkg].testsSkip++
				tr := ensureTestResult(collectors[pkg], ev.Test)
				tr.Status = "SKIP"
				tr.Elapsed = ev.Elapsed
				fmt.Printf("  [SKIP] %s (%.2fs)\n", ev.Test, ev.Elapsed)
				break
			}
			if stats[pkg].status == "" {
				stats[pkg].status = "skip"
				stats[pkg].elapsed = ev.Elapsed
				collectors[pkg].result.Status = "SKIP"
				collectors[pkg].result.Elapsed = ev.Elapsed
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
		pc := collectors[pkg]
		summary.testsRun += ps.testsRun
		summary.testsPass += ps.testsPass
		summary.testsFail += ps.testsFail
		summary.testsSkip += ps.testsSkip
		if ps.status == "" {
			if ps.testsFail > 0 {
				ps.status = "fail"
				pc.result.Status = "FAIL"
			} else if ps.testsSkip > 0 && ps.testsPass == 0 {
				ps.status = "skip"
				pc.result.Status = "SKIP"
			} else {
				ps.status = "pass"
				pc.result.Status = "PASS"
			}
		}
		if ps.status == "fail" {
			summary.packagesFail++
		} else {
			summary.packagesPass++
		}
		summary.packageResults = append(summary.packageResults, pc.result)
	}
	summary.failures = failures
	latestRunSummary = summary
	return nil
}

// testctl helper returning mutable test row, creating it if needed.
func ensureTestResult(pc *packageCollector, testName string) *testResult {
	if pc == nil {
		return &testResult{}
	}
	if idx, ok := pc.testIndex[testName]; ok {
		return &pc.result.Tests[idx]
	}
	idx := len(pc.result.Tests)
	pc.result.Tests = append(pc.result.Tests, testResult{Name: testName})
	pc.testIndex[testName] = idx
	return &pc.result.Tests[idx]
}

// testctl stderr forwarder for non-empty go test subprocess lines.
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

// testctl output formatter that filters noise and indents useful lines.
func renderOutputLine(raw string, withinTest bool) {
	display := strings.TrimSpace(raw)
	plain := sanitizeOutputLine(raw)
	if plain == "" {
		return
	}
	level := detectLogLevel(plain)
	if boundaryLinePrefix.MatchString(plain) {
		return
	}
	if plain == "PASS" || plain == "FAIL" {
		return
	}
	if packageLinePrefix.MatchString(plain) {
		return
	}
	if formatted, ok := formatTLVGetFieldLine(plain); ok {
		display = formatted
	} else if formatted, ok := formatMessageTypeLine(plain); ok {
		display = formatted
	} else if shouldSuppressOutputLine(plain) {
		return
	}
	display = applyLevelColor(display, level)
	prefix := "  |"
	if withinTest {
		prefix = "    |"
	}
	fmt.Printf("%s %s\n", prefix, display)
}

// sanitizeOutputLine strips ANSI color escapes and surrounding whitespace.
func sanitizeOutputLine(raw string) string {
	return strings.TrimSpace(ansiEscapePattern.ReplaceAllString(raw, ""))
}

// shouldSuppressOutputLine drops non-essential noisy log lines.
func shouldSuppressOutputLine(line string) bool {
	if strings.HasPrefix(line, "DEBUG ") {
		return true
	}
	if strings.HasPrefix(line, "DEV ") {
		return true
	}
	if strings.HasPrefix(line, "INFO ") {
		msg := strings.TrimPrefix(line, "INFO ")
		if strings.HasPrefix(msg, "test=") || strings.HasSuffix(msg, " ok") {
			return true
		}
	}
	return false
}

// formatTLVGetFieldLine rewrites tlv.GetField debug lines with descriptive field/type labels.
func formatTLVGetFieldLine(line string) (string, bool) {
	base := strings.TrimPrefix(line, "DEBUG ")
	base = strings.TrimPrefix(base, "DEV ")
	if !strings.HasPrefix(base, "tlv.GetField ") {
		return "", false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(base, "tlv.GetField "))
	switch {
	case strings.HasPrefix(payload, "found "):
		fields := parseKVFields(strings.TrimPrefix(payload, "found "))
		id, haveID := parseUint16Field(fields["id"])
		typeID, haveType := parseUint8Field(fields["type"])
		if !haveID {
			return "tlv.GetField found", true
		}
		out := fmt.Sprintf("tlv.GetField found field=%s", describeFieldID(id))
		if haveType {
			out += fmt.Sprintf(" type=%s", describeTLVType(typeID))
		}
		return out, true
	case strings.HasPrefix(payload, "missing "):
		fields := parseKVFields(strings.TrimPrefix(payload, "missing "))
		id, haveID := parseUint16Field(fields["id"])
		if !haveID {
			return "tlv.GetField missing", true
		}
		return fmt.Sprintf("tlv.GetField missing field=%s", describeFieldID(id)), true
	default:
		fields := parseKVFields(payload)
		id, haveID := parseUint16Field(fields["id"])
		count := strings.TrimSpace(fields["count"])
		if haveID && count != "" {
			return fmt.Sprintf("tlv.GetField lookup field=%s from=%s fields", describeFieldID(id), count), true
		}
		if haveID {
			return fmt.Sprintf("tlv.GetField lookup field=%s", describeFieldID(id)), true
		}
		return "tlv.GetField lookup", true
	}
}

// formatMessageTypeLine rewrites message_type numeric ids to descriptive envelope labels.
func formatMessageTypeLine(line string) (string, bool) {
	if !strings.Contains(line, "message_type=") {
		return "", false
	}
	tokens := strings.Fields(line)
	changed := false
	for i, token := range tokens {
		if !strings.HasPrefix(token, "message_type=") {
			continue
		}
		raw := strings.TrimPrefix(token, "message_type=")
		suffix := ""
		core := raw
		for len(core) > 0 {
			last := core[len(core)-1]
			if last >= '0' && last <= '9' {
				break
			}
			suffix = string(last) + suffix
			core = core[:len(core)-1]
		}
		if core == "" {
			continue
		}
		v, err := strconv.ParseUint(core, 10, 32)
		if err != nil {
			continue
		}
		tokens[i] = "message_type=" + describeMessageTypeID(uint32(v)) + suffix
		changed = true
	}
	if !changed {
		return "", false
	}
	return strings.Join(tokens, " "), true
}

func detectLogLevel(line string) string {
	switch {
	case strings.HasPrefix(line, "DEBUG "):
		return "DEBUG"
	case strings.HasPrefix(line, "DEV "):
		return "DEV"
	default:
		return ""
	}
}

func applyLevelColor(text string, level string) string {
	if strings.TrimSpace(text) == "" || ansiEscapePattern.MatchString(text) {
		return text
	}
	switch level {
	case "DEBUG":
		return ansiGreen + text + ansiReset
	case "DEV":
		return ansiMagenta + text + ansiReset
	default:
		return text
	}
}

// parseKVFields converts whitespace-delimited key=value tokens into a map.
func parseKVFields(payload string) map[string]string {
	out := make(map[string]string)
	for _, token := range strings.Fields(payload) {
		k, v, ok := strings.Cut(token, "=")
		if !ok {
			continue
		}
		out[k] = v
	}
	return out
}

// parseUint16Field parses a decimal uint16 value from a key-value field.
func parseUint16Field(raw string) (uint16, bool) {
	v, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 16)
	if err != nil {
		return 0, false
	}
	return uint16(v), true
}

// parseUint8Field parses a decimal uint8 value from a key-value field.
func parseUint8Field(raw string) (uint8, bool) {
	v, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 8)
	if err != nil {
		return 0, false
	}
	return uint8(v), true
}

// describeFieldID returns stable "name(id=n)" formatting for known/unknown schema field ids.
func describeFieldID(id uint16) string {
	if name, ok := fieldNameByID[id]; ok {
		return fmt.Sprintf("%s(id=%d)", name, id)
	}
	return fmt.Sprintf("field_%d(id=%d)", id, id)
}

// describeTLVType returns stable "name(id=n)" formatting for known/unknown tlv type ids.
func describeTLVType(typeID uint8) string {
	if name, ok := tlvTypeNameByID[typeID]; ok {
		return fmt.Sprintf("%s(id=%d)", name, typeID)
	}
	return fmt.Sprintf("type_%d(id=%d)", typeID, typeID)
}

// describeMessageTypeID returns stable "name(id=n)" formatting for known/unknown message type ids.
func describeMessageTypeID(messageType uint32) string {
	if name, ok := messageTypeNameByID[messageType]; ok {
		return fmt.Sprintf("%s(id=%d)", name, messageType)
	}
	return fmt.Sprintf("message_type_%d(id=%d)", messageType, messageType)
}

// testctl final printer for package/test matrix and aggregate totals.
func printRunSummary(start time.Time) {
	totalDuration := time.Since(start)
	fmt.Println()
	fmt.Println("Result Matrix")
	for _, pkg := range latestRunSummary.packageResults {
		fmt.Printf("  [%s] %s", pkg.Status, pkg.RelPath)
		if pkg.Elapsed > 0 {
			fmt.Printf(" (%.2fs)", pkg.Elapsed)
		}
		fmt.Println()
		if len(pkg.Tests) == 0 {
			fmt.Println("    - [NO-TESTS]")
			continue
		}
		for _, tr := range pkg.Tests {
			fmt.Printf("    - [%s] %s", tr.Status, tr.Name)
			if tr.Elapsed > 0 {
				fmt.Printf(" (%.2fs)", tr.Elapsed)
			}
			fmt.Println()
		}
	}

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

// testctl package-pattern normalizer for comma/space separated input.
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

// testctl helper returning current module import path.
func goListModulePath() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Path}}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// testctl helper resolving package patterns into deterministic import paths.
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

// testctl helper returning discovered test names for one package.
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

// testctl helper converting full import path to module-relative label.
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

// testctl helper mapping relative package path to stable reporting bucket.
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

// testctl helper returning deterministic map key order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// testctl fatal printer that exits non-zero.
func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "testctl: "+format+"\n", args...)
	os.Exit(1)
}
