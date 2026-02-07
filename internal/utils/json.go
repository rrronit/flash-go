package utils

import (
	"github.com/goccy/go-json"
	"flash-go/internal/models"

	"github.com/gin-gonic/gin"
)

func MarshalJob(job *models.Job) ([]byte, error) {
	return json.Marshal(job)
}

func UnmarshalJob(data []byte, job *models.Job) error {
	return json.Unmarshal(data, job)
}


func BindJSONFast(c *gin.Context, v interface{}) error {
	return json.NewDecoder(c.Request.Body).Decode(v)
}