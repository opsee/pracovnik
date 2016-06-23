package results

import (
	"github.com/opsee/basic/schema"
)

type Store interface {
	GetResultsByCheckId(string) ([]*schema.CheckResult, error)
	PutResult(*schema.CheckResult) error
}
