package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	deployedSHAFile     = ".deployed_sha"
	versionsHashFile    = ".versions_hash"
	previousImageIDFile = ".previous_image_id"
)

// DeployState holds the deployment state for an app.
type DeployState struct {
	DeployedSHA     string
	VersionsHash    string
	PreviousImageID string
}

// ReadState reads all deployment state files from the given directory.
func ReadState(dir string) DeployState {
	return DeployState{
		DeployedSHA:     readFile(dir, deployedSHAFile),
		VersionsHash:    readFile(dir, versionsHashFile),
		PreviousImageID: readFile(dir, previousImageIDFile),
	}
}

// WriteDeployedSHA records the currently deployed commit.
func WriteDeployedSHA(dir, sha string) error {
	return writeFile(dir, deployedSHAFile, sha)
}

// WriteVersionsHash records the hash of the .versions file.
func WriteVersionsHash(dir, hash string) error {
	return writeFile(dir, versionsHashFile, hash)
}

// WritePreviousImageID records the previous Docker image ID for rollback.
func WritePreviousImageID(dir, id string) error {
	return writeFile(dir, previousImageIDFile, id)
}

// ClearPreviousImageID removes the stored previous image ID.
func ClearPreviousImageID(dir string) error {
	path := filepath.Join(dir, previousImageIDFile)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", previousImageIDFile, err)
	}
	return nil
}

func readFile(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeFile(dir, name, content string) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
