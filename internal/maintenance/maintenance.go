package maintenance

import "github.com/fibegg/go-fibe/internal/models"

func Tasks() []models.MaintenanceTask {
	return []models.MaintenanceTask{
		{
			Name:        "rebuild_rollups",
			Description: "Clear dashboard rollups and force the next request to rebuild cached summaries.",
			Dangerous:   false,
		},
		{
			Name:        "clear_cache",
			Description: "Remove Redis-backed application caches.",
			Dangerous:   false,
		},
		{
			Name:        "prune_events",
			Description: "Delete check events older than 14 days.",
			Dangerous:   true,
		},
		{
			Name:        "reseed_demo",
			Description: "Ensure demo users and monitors exist.",
			Dangerous:   false,
		},
	}
}

func Exists(name string) bool {
	for _, task := range Tasks() {
		if task.Name == name {
			return true
		}
	}
	return false
}
