package packsync

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

// ValidateContent is the narrow Pack-content validation authority used at
// synchronization boundaries. It validates checked-in provenance, portable
// declarations, and referenced inert bytes without building or executing them.
func ValidateContent(bundleRoot string) error {
	configBytes, err := os.ReadFile(filepath.Join(bundleRoot, "sources.json"))
	if err != nil {
		return fmt.Errorf("read source configuration: %w", err)
	}
	config, err := LoadConfig(bytes.NewReader(configBytes))
	if err != nil {
		return err
	}
	if _, err := loadSourceLockSet(bundleRoot, config); err != nil {
		return fmt.Errorf("validate source configuration and lock bijection: %w", err)
	}
	if err := capabilitypack.ValidatePortableContent(bundleRoot); err != nil {
		return fmt.Errorf("validate portable Pack content: %w", err)
	}
	return nil
}
