package worker

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/opsee/basic/schema"
)

// GetState creates a State object populated by the check's settings and
// by the current state if it exists. If it the state is unknown, then it
// assumes a present state of OK.
func GetAndLockState(q sqlx.Ext, customerId, checkId string) (*State, error) {
	state := &State{}
	err := sqlx.Get(q, state, "SELECT states.state_id, states.customer_id, states.check_id, states.state_name, states.time_entered, states.last_updated, checks.min_failing_count, states.failing_count, states.response_count FROM check_states AS states JOIN checks ON (checks.id = states.check_id) WHERE states.customer_id = $1 AND checks.id = $2 FOR UPDATE OF states", customerId, checkId)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if err == sql.ErrNoRows {
		// Get the check so that we can get MinFailingCount and MinFailingTime
		// Return an error if the check doesn't exist
		// check, err := store.GetCheck(customerId, checkId)
		check := &schema.Check{}
		err := sqlx.Get(q, check, "SELECT id, customer_id, min_failing_count, min_failing_time FROM checks WHERE customer_id = $1 AND id = $2", customerId, checkId)
		if err != nil {
			return nil, err
		}

		state = &State{
			CheckId:         checkId,
			CustomerId:      customerId,
			Id:              StateOK,
			State:           StateOK.String(),
			TimeEntered:     time.Now(),
			LastUpdated:     time.Now(),
			MinFailingCount: check.MinFailingCount,
			MinFailingTime:  time.Duration(check.MinFailingTime) * time.Second,
			FailingCount:    0,
		}
	}

	state.MinFailingTime = state.MinFailingTime * time.Second

	return state, nil
}

func UpdateState(q sqlx.Ext, state *State) error {
	row := q.QueryRowx("SELECT sum(failing_count), sum(response_count) FROM check_state_memos WHERE check_id=$1 AND customer_id=$2", "check-id", "11111111-1111-1111-1111-111111111111")
	if err := row.Err(); err != nil {
		return err
	}
	var failingCount, responseCount int
	row.Scan(&failingCount, &responseCount)
	state.FailingCount = int32(failingCount)
	state.ResponseCount = int32(responseCount)

	return nil
}

func PutState(q sqlx.Ext, state *State) error {
	_, err := sqlx.NamedExec(q, "INSERT INTO check_states (check_id, customer_id, state_id, state_name, time_entered, last_updated, failing_count, response_count) VALUES (:check_id, :customer_id, :state_id, :state_name, :time_entered, :last_updated, :failing_count, :response_count) ON CONFLICT (check_id) DO UPDATE SET state_id = :state_id, state_name = :state_name, time_entered = :time_entered, last_updated = :last_updated, failing_count = :failing_count, response_count = :response_count", state)
	if err != nil {
		return err
	}

	return nil
}

func PutMemo(q sqlx.Ext, memo *ResultMemo) error {
	_, err := sqlx.NamedExec(q, "INSERT INTO check_state_memos AS csm (check_id, customer_id, bastion_id, failing_count, response_count, last_updated) VALUES (:check_id, :customer_id, :bastion_id, :failing_count, :response_count, :last_updated) ON CONFLICT (check_id, bastion_id) DO UPDATE SET failing_count = :failing_count, response_count = :response_count, last_updated = :last_updated WHERE csm.check_id = :check_id AND csm.bastion_id = :bastion_id", memo)
	if err != nil {
		return err
	}

	return nil
}

func GetMemo(q sqlx.Ext, checkId, bastionId string) (*ResultMemo, error) {
	memo := &ResultMemo{}
	err := sqlx.Get(q, memo, "SELECT * FROM check_state_memos WHERE check_id = $1 AND bastion_id = $2 LIMIT 1", checkId, bastionId)
	if err != nil {
		return nil, err
	}

	return memo, nil
}
