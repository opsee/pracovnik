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

	state, err := GetState(w.result.CustomerId, w.result.CheckId)
	if err != nil {
		logger.WithError(err).Error("Error getting state.")
		return nil, err
	}

	latestMemo, ok := state.Results[w.result.BastionId]
	if ok {
		// We've seen this bastion before, and we have a newer result so we don't
		// transition. In any other case, we transition.
		if latestMemo.LastUpdate > w.result.Timestamp.Millis() {
			return nil, nil
		}
	}

	if err := state.Transition(w.result); err != nil {
		logger.WithError(err).Error("Error transitioning state.")
		return nil, err
	}

	if err := PutState(state); err != nil {
		logger.WithError(err).Error("Error storing state.")
		return nil, err
	}

	return nil, nil
}
