package mysql

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"task-service/internal/domain"
)

// Агрегаты считаются отдельными ветками одного запроса.
// Общий JOIN задач и комментариев умножил бы число задач на число комментариев.
const statsSQL = `
SELECT kind, dim, val FROM (
    SELECT 'status' COLLATE utf8mb4_unicode_ci AS kind,
           status COLLATE utf8mb4_unicode_ci AS dim,
           CAST(COUNT(*) AS DECIMAL(20,4)) AS val,
           0 AS ord
    FROM tasks
    WHERE team_id = ?
    GROUP BY status

    UNION ALL

    SELECT 'top' COLLATE utf8mb4_unicode_ci,
           CAST(assignee_id AS CHAR) COLLATE utf8mb4_unicode_ci,
           CAST(closed_count AS DECIMAL(20,4)),
           rn
    FROM (
        SELECT assignee_id,
               COUNT(*) AS closed_count,
               ROW_NUMBER() OVER (ORDER BY COUNT(*) DESC, assignee_id ASC) AS rn
        FROM tasks
        WHERE team_id = ?
          AND assignee_id IS NOT NULL
          AND closed_at IS NOT NULL
          AND closed_at >= ?
        GROUP BY assignee_id
    ) ranked
    WHERE rn <= 3

    UNION ALL

    SELECT 'avg' COLLATE utf8mb4_unicode_ci,
           '' COLLATE utf8mb4_unicode_ci,
           CAST(AVG(TIMESTAMPDIFF(SECOND, created_at, closed_at)) AS DECIMAL(20,4)),
           0
    FROM tasks
    WHERE team_id = ?
      AND closed_at IS NOT NULL

    UNION ALL

    SELECT 'comments' COLLATE utf8mb4_unicode_ci,
           '' COLLATE utf8mb4_unicode_ci,
           CAST(COUNT(c.id) AS DECIMAL(20,4)),
           0
    FROM tasks t
    LEFT JOIN task_comments c ON c.task_id = t.id
    WHERE t.team_id = ?
) report
ORDER BY kind, ord`

func (r *Repository) Stats(ctx context.Context, teamID int64, since time.Time) (domain.Stats, error) {
	rows, err := r.db.QueryContext(ctx, statsSQL, teamID, teamID, since.UTC(), teamID, teamID)
	if err != nil {
		return domain.Stats{}, err
	}
	defer rows.Close()

	stats := domain.EmptyStats()
	top := make([]domain.TopAssignee, 0, 3)
	for rows.Next() {
		var kind, dim string
		var value sqlNullFloat
		if err := rows.Scan(&kind, &dim, &value); err != nil {
			return domain.Stats{}, err
		}
		switch kind {
		case "status":
			if domain.ValidStatus(dim) {
				stats.TasksByStatus[dim] = int(value.Float64)
			}
		case "top":
			assigneeID, err := strconv.ParseInt(dim, 10, 64)
			if err != nil {
				return domain.Stats{}, err
			}
			top = append(top, domain.TopAssignee{AssigneeID: assigneeID, ClosedCount: int(value.Float64)})
		case "avg":
			if value.Valid {
				avg := value.Float64
				stats.AvgCloseSeconds = &avg
			}
		case "comments":
			stats.CommentCount = int(value.Float64)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.Stats{}, err
	}
	stats.TopAssignees = top
	return stats, nil
}

type sqlNullFloat struct {
	Float64 float64
	Valid   bool
}

func (v *sqlNullFloat) Scan(src any) error {
	if src == nil {
		v.Float64 = 0
		v.Valid = false
		return nil
	}
	switch typed := src.(type) {
	case []byte:
		parsed, err := strconv.ParseFloat(string(typed), 64)
		if err != nil {
			return err
		}
		v.Float64 = parsed
		v.Valid = true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return err
		}
		v.Float64 = parsed
		v.Valid = true
	case float64:
		v.Float64 = typed
		v.Valid = true
	case float32:
		v.Float64 = float64(typed)
		v.Valid = true
	case int64:
		v.Float64 = float64(typed)
		v.Valid = true
	default:
		return fmt.Errorf("unexpected stats value %T", src)
	}
	return nil
}
