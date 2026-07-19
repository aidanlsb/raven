package index

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
	if err := d.db.QueryRow("SELECT COUNT(*) FROM assets").Scan(&stats.AssetCount); err != nil {
		return nil, err
	}
	if err := d.db.QueryRow(`
		SELECT COUNT(DISTINCT file_path)
		FROM (
			SELECT file_path FROM objects
			UNION
			SELECT file_path FROM assets
		)
	`).Scan(&stats.FileCount); err != nil {
		return nil, err
	}

	return &stats, nil
}

// IndexStats contains index statistics.
type IndexStats struct {
	ObjectCount int
	TraitCount  int
	RefCount    int
	FileCount   int
	AssetCount  int
}
