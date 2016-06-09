package worker

import (
	"github.com/opsee/basic/schema"
)

type ResultStore interface {
	GetResults(*schema.CheckResult) (map[string]*schema.CheckResult, error)
	PutResult(*schema.CheckResult) error
}
