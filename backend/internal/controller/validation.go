package controller

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/gin-gonic/gin/binding"
)

func parseTaskInput(raw map[string]any, in *service.TaskInput) error {
	b, e := json.Marshal(raw)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, in); e != nil {
		return e
	}
	if e = binding.Validator.ValidateStruct(in); e != nil {
		return e
	}
	if strings.TrimSpace(in.Title) == "" {
		return errors.New("empty title")
	}
	return nil
}