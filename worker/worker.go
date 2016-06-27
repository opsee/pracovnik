package worker

import (
	"database/sql"
	"time"

	log "github.com/opsee/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/opsee/basic/schema"
	"github.com/opsee/pracovnik/results"
	"golang.org/x/net/context"
)

var (
	logger = log.WithFields(log.Fields{
		"worker": "check_worker",
	})
)

type CheckWorker struct {
	db      *sqlx.DB
	rStore  results.Store
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

func NewCheckWorker(db *sqlx.DB, rStore results.Store, result *schema.CheckResult) *CheckWorker {
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
	logger.Debug("Handling check result")

	tx, err := w.db.Beginx()
	if err != nil {
		logger.WithError(err).Error("Cannot open transaction.")
		return nil, err
	}

	memo, err := GetMemo(tx, w.result.CheckId, w.result.BastionId)
	if err != nil && err != sql.ErrNoRows {
		logger.WithError(err).Error("Unable to get check state memo from DB.")
		rollback(logger, tx)
		return nil, err
	}
	if err == sql.ErrNoRows {
		memo = ResultMemoFromCheckResult(w.result)
	}

	resultTimestamp := time.Unix(w.result.Timestamp.Seconds, int64(w.result.Timestamp.Nanos))
	// We've seen this bastion before, and we have a newer result so we don't
	// transition. In any other case, we transition.
	//
	// TODO(greg): When we have historical results, this will still have to be
	// put into the cold dynamodb table.
	if memo.LastUpdated.After(resultTimestamp) {
		logger.Debug("Skipping older result because we have a newer result memo.")
		rollback(logger, tx)
		return nil, nil
	}

	memo.FailingCount = int32(w.result.FailingCount())
	memo.ResponseCount = len(w.result.Responses)

	if err := PutMemo(tx, memo); err != nil {
		logger.Debug("Error putting check state memo.")
		rollback(logger, tx)
		return nil, err
	}
	logger.Debug("Put memo: ", memo)

	state, err := GetAndLockState(tx, w.result.CustomerId, w.result.CheckId)
	if err != nil {
		logger.WithError(err).Error("Error getting state.")
		rollback(logger, tx)
		return nil, err
	}
	logger.Debug("Got state: ", state)

	if err := UpdateState(tx, state); err != nil {
		logger.Debug("Error updating state from DB.")
		rollback(logger, tx)
		return nil, err
	}
	logger.Debug("Updated state: ", state)

	if err := state.Transition(w.result); err != nil {
		logger.WithError(err).Error("Error transitioning state.")
		rollback(logger, tx)
		return nil, err
	}
	logger.Debug("State after transition: ", state)

	if err := PutState(tx, state); err != nil {
		logger.WithError(err).Error("Error storing state.")
		rollback(logger, tx)
		return nil, err
	}
	logger.Debug("State after put state: ", state)

	// still try to store the result even if we couldn't transition
	// check state?
	// TODO(greg): should we do this? should we do something else?

	if err := commit(logger, tx); err != nil {
		logger.WithError(err).Error("Could not commit check state.")
	}
	logger.Debug("committed state.")

	if err := w.rStore.PutResult(w.result); err != nil {
		logger.WithError(err).Error("Error putting CheckResult to dynamodb.")
		return nil, err
	}

	return nil, nil
}
