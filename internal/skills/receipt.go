package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
)

const receiptFileName = ".rvn-skill-receipt.json"

type Receipt struct {
	Skill       string   `json:"skill"`
	Version     int      `json:"version"`
	Scope       string   `json:"scope"`
	Checksum    string   `json:"checksum"`
	Files       []string `json:"files"`
	InstalledAt string   `json:"installed_at"`
}

func newReceipt(skillID string, version int, scope string, rendered map[string][]byte) *Receipt {
	return &Receipt{
		Skill:       skillID,
		Version:     version,
		Scope:       scope,
		Checksum:    checksumForRendered(rendered),
		Files:       sortedRenderedPaths(rendered),
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func writeReceipt(path string, receipt *Receipt) error {
	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal receipt: %w", err)
	}
	if err := atomicfile.WriteFile(path, receiptBytes, 0o644); err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}
	return nil
}

func readReceipt(path string) (*Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func checksumForRendered(rendered map[string][]byte) string {
	h := sha256.New()
	paths := sortedRenderedPaths(rendered)
	for _, rel := range paths {
		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(rendered[rel])
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func receiptMatchesRendered(receipt *Receipt, skill *Skill, scope string, rendered map[string][]byte) bool {
	if receipt == nil || skill == nil {
		return false
	}
	return receipt.Checksum == checksumForRendered(rendered) &&
		receipt.Skill == skill.Spec.ID &&
		receipt.Version == skill.Spec.Version &&
		receipt.Scope == scope
}
