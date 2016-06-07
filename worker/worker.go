package worker

import (
	log "github.com/Sirupsen/logrus"
	"github.com/opsee/basic/schema"
	"golang.org/x/net/context"
)

var (
	logger = log.WithFields(log.Fields{
		"worker": "check_worker",
	})
)

type CheckWorker struct {
	context context.Context
	result  *schema.CheckResult
}

func NewCheckWorker(result *schema.CheckResult) *CheckWorker {
	return &CheckWorker{
		context: context.Background(),
		result:  result,
	}
}

func (w *CheckWorker) Context() context.Context {
	return w.context
}

func (w *CheckWorker) Execute() (interface{}, error) {
	logger = logger.WithFields(log.Fields{
		"check_id":    w.result.CheckId,
		"customer_id": w.result.CustomerId,
		"bastion_id":  w.result.BastionId,
	})

	logger.Debug("Handling check result: ", w.result)

	if err := PutResult(w.result); err != nil {
		logger.WithError(err).Error("Error putting CheckResult to dynamodb.")
	}

	// TODO(greg): manage state transitions
	/*
		// state := GetState(result.CheckId)
		state := &state{}

		if err := StoreState(transition(state, result)); err != nil {
			logger.WithError(err).Error("Unable to store state.")
		}
	*/

	// return map[string]interface{}{"ok": true}, nil
	return nil, nil
}
