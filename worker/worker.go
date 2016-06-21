package worker

import (
	"database/sql"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/opsee/basic/schema"
	"golang.org/x/net/context"
)

var (
	logger = log.WithFields(log.Fields{
		"worker": "check_worker",
	})
)

type CheckWorker struct {
	db      *sqlx.DB
	rStore  ResultStore
	context context.Context
	result  *schema.CheckResult
}

func rollback(logger log.FieldLogger, tx *sqlx.Tx) error {
	err := tx.Rollback()
	if err != nil {
		logger.WithError(err).Error("Error rolling back transaction.")
	}
	return err
}

func commit(logger log.FieldLogger, tx *sqlx.Tx) error {
	err := tx.Commit()
	if err != nil {
		logger.WithError(err).Error("Error committing transaction.")
	}
	return err
}

func NewCheckWorker(db *sqlx.DB, rStore ResultStore, result *schema.CheckResult) *CheckWorker {
	return &CheckWorker{
		db:      db,
		rStore:  rStore,
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

	tx, err := w.db.Beginx()
	if err != nil {
		logger.WithError(err).Error("Cannot open transaction.")
		return nil, err
	}

	logger.Debug("Handling check result: ", w.result)

	if err := w.rStore.PutResult(w.result); err != nil {
		logger.WithError(err).Error("Error putting CheckResult to dynamodb.")
		rollback(logger, tx)
		return nil, err
	}

	state, err := GetState(tx, w.result.CustomerId, w.result.CheckId)
	if err != nil {
		logger.WithError(err).Error("Error getting state.")
		rollback(logger, tx)

		if err == sql.ErrNoRows {
			return nil, nil
		}

		return nil, err
	}

	latestMemo, ok := state.Results[w.result.BastionId]
	if ok {
		resultTimestamp := time.Unix(w.result.Timestamp.Seconds, int64(w.result.Timestamp.Nanos))
		// We've seen this bastion before, and we have a newer result so we don't
		// transition. In any other case, we transition.
		if latestMemo.LastUpdated.After(resultTimestamp) {
			commit(logger, tx)
			return nil, nil
		}
	}

	if err := state.Transition(w.result); err != nil {
		logger.WithError(err).Error("Error transitioning state.")
		rollback(logger, tx)
		return nil, err
	}

	// This is weird here, but there's not a lot we can really do about it.
	// TODO(greg): this should all be refactored a bit to be less meh.
	memo := state.Results[w.result.BastionId]
	if err := PutMemo(tx, memo); err != nil {
		logger.WithError(err).Errorf("Error storing memo: %q", memo)
		rollback(logger, tx)
		return nil, err
	}

	if err := PutState(tx, state); err != nil {
		logger.WithError(err).Error("Error storing state.")
		rollback(logger, tx)
		return nil, err
	}

	return nil, commit(logger, tx)
}
