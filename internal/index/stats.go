package index

import "fmt"

// Stats returns statistics about the index.
func (d *Database) Stats() (*IndexStats, error) {
	var stats IndexStats

	if err := d.db.QueryRow("SELECT COUNT(*) FROM objects").Scan(&stats.ObjectCount); err != nil {
		return nil, err
	}
	if err := d.db.QueryRow("SELECT COUNT(*) FROM traits").Scan(&stats.TraitCount); err != nil {
		return nil, err
	}
	if err := d.db.QueryRow("SELECT COUNT(*) FROM refs").Scan(&stats.RefCount); err != nil {
		return nil, err
	}
	if err := d.db.QueryRow("SELECT COUNT(DISTINCT file_path) FROM objects").Scan(&stats.FileCount); err != nil {
		return nil, err
	}

	return &stats, nil
}

// StatsForFile returns object, trait, and reference statistics for a file.
func (d *Database) StatsForFile(filePath string) (IndexStats, error) {
	if d == nil || d.db == nil {
		return IndexStats{}, fmt.Errorf("database is nil")
	}

	var stats IndexStats
	if err := d.db.QueryRow("SELECT COUNT(*) FROM objects WHERE file_path = ?", filePath).Scan(&stats.ObjectCount); err != nil {
		return IndexStats{}, err
	}
	if err := d.db.QueryRow("SELECT COUNT(*) FROM traits WHERE file_path = ?", filePath).Scan(&stats.TraitCount); err != nil {
		return IndexStats{}, err
	}
	if err := d.db.QueryRow("SELECT COUNT(*) FROM refs WHERE file_path = ?", filePath).Scan(&stats.RefCount); err != nil {
		return IndexStats{}, err
	}
	return stats, nil
}

// IndexStats contains index statistics.
type IndexStats struct {
	ObjectCount int
	TraitCount  int
	RefCount    int
	FileCount   int
}
