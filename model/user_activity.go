package model

func filterInactiveUserIDs(candidateIDs []int, since int64) ([]int, error) {
	inactive := make([]int, 0, len(candidateIDs))
	// Keep the number of SQL bind parameters bounded on SQLite and PostgreSQL.
	for start := 0; start < len(candidateIDs); start += 500 {
		batch := candidateIDs[start:min(start+500, len(candidateIDs))]
		var activeIDs []int
		if err := LOG_DB.Model(&Log{}).Where("user_id IN ? AND created_at >= ? AND type IN ?", batch, since, []int{LogTypeConsume, LogTypeError}).Distinct("user_id").Pluck("user_id", &activeIDs).Error; err != nil {
			return nil, err
		}
		active := make(map[int]bool, len(activeIDs))
		for _, id := range activeIDs {
			active[id] = true
		}
		for _, id := range batch {
			if !active[id] {
				inactive = append(inactive, id)
			}
		}
	}
	return inactive, nil
}
