package service

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// stateFile is the on-disk shape of the app's small session state.
type stateFile struct {
	LastRepo string `json:"lastRepo"`
}

// statePath returns the file that remembers the last opened repository.
// MyGo's PathUserData is the per-app directory of the platform, created on
// first use: ~/Library/Application Support/<app name> on macOS.
func statePath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// OpenInitial opens arg (a repository or a file inside one) or, when arg is
// empty, the repository the previous session left open. A failed restore is
// logged instead of shown, so a moved or unmounted project cannot block
// startup.
func OpenInitial(s *RepoService, arg string) {
	path, quiet := arg, false
	if path == "" {
		path, quiet = loadLastRepo(), true
	}
	if path == "" {
		return
	}
	s.openPath(path, quiet)
}

// saveLastRepo records root so the next launch reopens it. Failures are
// logged, never surfaced: a read-only config directory must not stop the
// repository from opening.
func saveLastRepo(root string) {
	path, err := statePath()
	if err != nil {
		log.Printf("service: state path unavailable: %v", err)
		return
	}
	if err := writeLastRepo(path, root); err != nil {
		log.Printf("service: save last repository %q: %v", root, err)
	}
}

// loadLastRepo returns the saved repository root, or "" when there is none.
func loadLastRepo() string {
	path, err := statePath()
	if err != nil {
		return ""
	}
	return readLastRepo(path)
}

// readLastRepo returns "" for a missing file, an unreadable file, or invalid
// JSON, so a damaged state file only means "nothing to restore".
func readLastRepo(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var state stateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return ""
	}
	return state.LastRepo
}

// writeLastRepo writes state through a temporary file and a rename, so a
// crash mid-write leaves the previous state file intact.
func writeLastRepo(path, root string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(stateFile{LastRepo: root})
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
