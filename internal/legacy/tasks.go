package legacy

import (
	"context"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UpdateTaskForUserToDisplay ports user_task/handler.js updateTaskForUserToDisplay for the BAH task names.
// It closes the user's open task for the master and, for build_a_habit, closes duplicates.
func (s *Service) UpdateTaskForUserToDisplay(ctx context.Context, userID, caseID, taskName string) error {
	master, err := s.Store.FindTaskMasterByName(ctx, taskName)
	if err != nil || master == nil || master.ID.IsZero() {
		return err
	}
	task, err := s.Store.FindOpenUserTask(ctx, userID, master.ID)
	if err != nil || task == nil || task.ID.IsZero() {
		return err // "No task to update"
	}
	if task.UserID != userID {
		return nil
	}
	clickCount := task.ClickCount + 1
	now := s.now()
	if taskName == TaskBuildAHabit {
		var ids []primitive.ObjectID
		for _, raw := range BahTaskIDs {
			if oid, err := primitive.ObjectIDFromHex(raw); err == nil {
				ids = append(ids, oid)
			}
		}
		if len(ids) > 0 {
			if _, err := s.Store.CloseDuplicateOpenTasks(ctx, userID, ids, task.ID, now); err != nil {
				s.Log.Warn("closing duplicate build_a_habit tasks failed", "userId", userID, "error", err.Error())
			}
		}
	}
	return s.Store.CompleteUserTask(ctx, task.ID, TaskCompletionCustomer, TaskCompletionCustomer, clickCount, now)
}
