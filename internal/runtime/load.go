package runtime

import (
	"encoding/json"
	"litcontainer/internal/logger"
	"os"
	"path/filepath"
)

func LoadSpec(bundleDir string) (*Spec, error) {
	content, err := os.ReadFile(filepath.Join(bundleDir, DefaultSpecFileName))
	if err != nil {
		logger.Error("failed to read spec file: %v", err)
		return nil, err
	}

	var spec Spec
	err = json.Unmarshal(content, &spec)
	if err != nil {
		logger.Error("failed to unmarshal spec: %v", err)
		return nil, err
	}

	return &spec, nil
}
