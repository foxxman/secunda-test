package domain

import (
	"sort"
	"time"
)

func ComputeStats(tasks []Task, comments []Comment, teamID int64, since time.Time) Stats {
	stats := EmptyStats()
	byAssignee := map[int64]int{}
	var closeSum float64
	var closed int
	teamTasks := map[int64]struct{}{}

	for _, task := range tasks {
		if task.TeamID != teamID {
			continue
		}
		teamTasks[task.ID] = struct{}{}
		if ValidStatus(task.Status) {
			stats.TasksByStatus[task.Status]++
		}
		if task.ClosedAt == nil {
			continue
		}
		closed++
		closeSum += float64(task.ClosedAt.Sub(task.CreatedAt) / time.Second)
		if task.AssigneeID != nil && !task.ClosedAt.Before(since) {
			byAssignee[*task.AssigneeID]++
		}
	}
	if closed > 0 {
		avg := closeSum / float64(closed)
		stats.AvgCloseSeconds = &avg
	}
	for _, comment := range comments {
		if _, ok := teamTasks[comment.TaskID]; ok {
			stats.CommentCount++
		}
	}

	top := make([]TopAssignee, 0, len(byAssignee))
	for assigneeID, count := range byAssignee {
		top = append(top, TopAssignee{AssigneeID: assigneeID, ClosedCount: count})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].ClosedCount == top[j].ClosedCount {
			return top[i].AssigneeID < top[j].AssigneeID
		}
		return top[i].ClosedCount > top[j].ClosedCount
	})
	if len(top) > 3 {
		top = top[:3]
	}
	stats.TopAssignees = top
	return stats
}
