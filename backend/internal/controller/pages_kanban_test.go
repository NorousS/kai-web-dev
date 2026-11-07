package controller

import (
	"testing"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
)

func TestGroupProjectTasks(t *testing.T) {
	input := []PageTask{
		{TaskView: service.TaskView{Task: model.Task{ID: 3, Status: "in_review"}}},
		{TaskView: service.TaskView{Task: model.Task{ID: 1, Status: "todo"}}},
		{TaskView: service.TaskView{Task: model.Task{ID: 2, Status: "todo"}}},
		{TaskView: service.TaskView{Task: model.Task{ID: 4, Status: "done"}}},
	}
	columns := groupProjectTasks(input)
	if len(columns) != 4 {
		t.Fatalf("expected four columns, got %d", len(columns))
	}
	wantKeys := []string{"todo", "in_progress", "in_review", "done"}
	wantCounts := []int{2, 0, 1, 1}
	for i, col := range columns {
		if col.Key != wantKeys[i] || len(col.Tasks) != wantCounts[i] {
			t.Errorf("column %d: %q, %d tasks", i, col.Key, len(col.Tasks))
		}
	}
	if columns[0].Tasks[0].ID != 1 || columns[0].Tasks[1].ID != 2 {
		t.Fatal("tasks must retain original order within columns")
	}
	for i, col := range groupProjectTasks(nil) {
		if len(col.Tasks) != 0 {
			t.Errorf("empty board column %d should remain empty", i)
		}
	}
}