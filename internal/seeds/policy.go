package seeds

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Safe token for values passed as git/brew arguments; a leading '-' would be parsed as an option.
var safeArgPattern = regexp.MustCompile(`^[A-Za-z0-9@._+/][A-Za-z0-9@._+/-]*$`)

// Install-policy errors (docs/progress/mvp_p8.md Slice F, mvp_p9.md dependency model).
var (
	// Method is not on the github|curl|brew|stream allowlist.
	ErrInstallMethodNotAllowed = errors.New("seeds: install method not allowlisted")
	// stream transfer must flow mirage -> ghost.
	ErrInstallTransferDirection = errors.New("seeds: stream transfer direction must be mirage->ghost")
	// Method is allowlisted but its transfer is not implemented yet (stream, Phase 9).
	ErrInstallMethodDeferred = errors.New("seeds: install method not implemented yet")
)

// StreamDirectionMirageToGhost is the only permitted direction for stream transfers.
const StreamDirectionMirageToGhost = "mirage->ghost"

// AllowedInstallMethods is the install-method allowlist enforced at every runtime boundary.
var AllowedInstallMethods = []InstallMethod{
	InstallMethodGitHub,
	InstallMethodCurl,
	InstallMethodBrew,
	InstallMethodStream,
}

// MethodAllowed reports whether m is on the allowlist (case-insensitive).
func MethodAllowed(m InstallMethod) bool {
	norm := InstallMethod(strings.ToLower(strings.TrimSpace(string(m))))
	for _, allowed := range AllowedInstallMethods {
		if norm == allowed {
			return true
		}
	}
	return false
}

// ValidateSpec enforces install policy for one spec independent of any host state:
// allowlisted method, pinned verified source per method, mirage->ghost stream direction, and a
// relative destination that cannot escape the project-managed install root. brew is the single
// host-global exception; its binary reaches Ghost only through a symlink in the managed bin root.
func ValidateSpec(spec InstallSpec) error {
	if strings.TrimSpace(spec.SeedID) == "" {
		return fmt.Errorf("%w: missing seed_id", ErrInstallInvalidSpec)
	}
	method := InstallMethod(strings.ToLower(strings.TrimSpace(string(spec.Method))))
	if !MethodAllowed(method) {
		return fmt.Errorf("%w: %q", ErrInstallMethodNotAllowed, spec.Method)
	}
	if err := validateRelativeDestination(spec.Destination); err != nil {
		return err
	}
	switch method {
	case InstallMethodGitHub:
		if err := validateGitHubRepo(strings.TrimSpace(spec.RepoURL)); err != nil {
			return err
		}
		if strings.TrimSpace(spec.Ref) == "" {
			return fmt.Errorf("%w: github install requires a pinned ref", ErrInstallInvalidSpec)
		}
		if err := validateArg("ref", spec.Ref); err != nil {
			return err
		}
		if strings.TrimSpace(spec.Branch) != "" {
			if err := validateArg("branch", spec.Branch); err != nil {
				return err
			}
		}
	case InstallMethodCurl:
		if err := validateHTTPSURL(spec.URL); err != nil {
			return err
		}
		if strings.TrimSpace(spec.Version) == "" {
			return fmt.Errorf("%w: curl install requires version", ErrInstallInvalidSpec)
		}
		if err := validateSHA256(spec.SHA256); err != nil {
			return err
		}
		if strings.TrimSpace(spec.Destination) == "" {
			return fmt.Errorf("%w: curl install requires destination file", ErrInstallInvalidSpec)
		}
	case InstallMethodBrew:
		if strings.TrimSpace(spec.Package) == "" {
			return fmt.Errorf("%w: brew install requires package", ErrInstallInvalidSpec)
		}
		if err := validateArg("package", spec.Package); err != nil {
			return err
		}
		if strings.TrimSpace(spec.Tap) != "" {
			if err := validateArg("tap", spec.Tap); err != nil {
				return err
			}
		}
	case InstallMethodStream:
		if strings.TrimSpace(spec.Direction) != StreamDirectionMirageToGhost {
			return fmt.Errorf("%w: got %q", ErrInstallTransferDirection, spec.Direction)
		}
		if err := validateSHA256(spec.SHA256); err != nil {
			return err
		}
	}
	return nil
}

// Destination must be relative and stay inside whatever install root it is joined to.
func validateRelativeDestination(dest string) error {
	d := strings.TrimSpace(dest)
	if d == "" {
		return nil
	}
	if filepath.IsAbs(d) {
		return fmt.Errorf("%w: destination must be relative: %q", ErrInstallSandboxViolation, dest)
	}
	clean := filepath.Clean(d)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: destination escapes install root: %q", ErrInstallSandboxViolation, dest)
	}
	return nil
}

// Rejects values that could be read as command options or contain shell/whitespace characters.
func validateArg(name string, value string) error {
	if !safeArgPattern.MatchString(strings.TrimSpace(value)) {
		return fmt.Errorf("%w: unsafe %s %q", ErrInstallInvalidSpec, name, value)
	}
	return nil
}

func validateHTTPSURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w: curl url must be https: %q", ErrInstallInvalidSpec, raw)
	}
	return nil
}

func validateSHA256(sum string) error {
	s := strings.ToLower(strings.TrimSpace(sum))
	if len(s) != 64 {
		return fmt.Errorf("%w: sha256 must be 64 hex characters", ErrInstallInvalidSpec)
	}
	if _, err := hex.DecodeString(s); err != nil {
		return fmt.Errorf("%w: sha256 is not hex", ErrInstallInvalidSpec)
	}
	return nil
}
