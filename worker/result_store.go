package worker

import (
	"github.com/opsee/basic/schema"
)

type ResultStore interface {
	GetResultsByCheckId(string) (map[string]*schema.CheckResult, error)
	PutResult(*schema.CheckResult) error
}
