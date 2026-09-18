package mongorepo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"traya-bah-service/models"
)

// FindTaskMasterByName finds an active task master by task_name.
func (s *Store) FindTaskMasterByName(ctx context.Context, name string) (*models.TaskMaster, error) {
	var out models.TaskMaster
	if err := s.C(CollTaskMaster).FindOne(ctx, bson.M{"task_name": name, "is_active": true}).Decode(&out); err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

var openTaskFilter = bson.M{"is_active": true, "completed_date": bson.M{"$exists": false}}

// FindOpenUserTask finds the user's open (active, never completed) task detail for a master.
func (s *Store) FindOpenUserTask(ctx context.Context, userID string, taskID primitive.ObjectID) (*models.UserTaskDetail, error) {
	f := bson.M{"task_id": taskID, "user_id": userID}
	for k, v := range openTaskFilter {
		f[k] = v
	}
	var out models.UserTaskDetail
	if err := s.C(CollUserTaskDetails).FindOne(ctx, f).Decode(&out); err != nil {
		if none, err := noDoc(err); none || err != nil {
			return nil, err
		}
	}
	return &out, nil
}

// CompleteUserTask mirrors the $set in updateTaskForUserToDisplay.
func (s *Store) CompleteUserTask(ctx context.Context, id primitive.ObjectID, completedBy, completionResponse string, clickCount int, now time.Time) error {
	set := bson.M{"is_active": false, "completed_date": now, "completed_by": completedBy, "click_count": clickCount, "updatedAt": now}
	if completionResponse != "" {
		set["task_completion_response"] = completionResponse
	}
	_, err := s.C(CollUserTaskDetails).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// CloseDuplicateOpenTasks closes other open tasks for the given masters (completed_by SYSTEM).
func (s *Store) CloseDuplicateOpenTasks(ctx context.Context, userID string, masterIDs []primitive.ObjectID, except primitive.ObjectID, now time.Time) (int64, error) {
	f := bson.M{"user_id": userID, "task_id": bson.M{"$in": masterIDs}, "_id": bson.M{"$ne": except}}
	for k, v := range openTaskFilter {
		f[k] = v
	}
	res, err := s.C(CollUserTaskDetails).UpdateMany(ctx, f, bson.M{"$set": bson.M{"is_active": false, "completed_date": now, "completed_by": "SYSTEM", "updatedAt": now}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

// CreateUserTask mirrors createTaskForUserToDisplay's UserTaskDetail.create.
func (s *Store) CreateUserTask(ctx context.Context, doc *models.UserTaskDetail) error {
	now := s.Clock.Now()
	doc.CreatedAt, doc.UpdatedAt = now, now
	_, err := s.C(CollUserTaskDetails).InsertOne(ctx, doc, options.InsertOne())
	return err
}
