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
func GetState(q sqlx.Ext, customerId, checkId string) (*State, error) {
	state := &State{}
	err := sqlx.Get(q, state, "SELECT states.state_id, states.customer_id, states.check_id, states.state_name, states.time_entered, states.last_updated, checks.min_failing_count, checks.min_failing_time, states.failing_count FROM check_states AS states JOIN checks ON (checks.id = states.check_id) WHERE states.customer_id = $1 AND checks.id = $2", customerId, checkId)
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
			State:           StateStrings[StateOK],
			TimeEntered:     time.Now(),
			LastUpdated:     time.Now(),
			MinFailingCount: check.MinFailingCount,
			MinFailingTime:  time.Duration(check.MinFailingTime) * time.Second,
			FailingCount:    0,
		}
	}
	state.Results = map[string]*ResultMemo{}
	state.MinFailingTime = state.MinFailingTime * time.Second

	memos := []*ResultMemo{}
	err = sqlx.Select(q, &memos, "SELECT * FROM check_state_memos WHERE customer_id = $1 AND check_id = $2", customerId, checkId)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	for _, memo := range memos {
		state.Results[memo.BastionId] = memo
	}

	return state, nil
}

func PutState(q sqlx.Ext, state *State) error {
	_, err := sqlx.NamedExec(q, "INSERT INTO check_states (check_id, customer_id, state_id, state_name, time_entered, last_updated, failing_count, response_count) VALUES (:check_id, :customer_id, :state_id, :state_name, :time_entered, :last_updated, :failing_count, :response_count) ON CONFLICT (check_id) DO UPDATE SET state_id = :state_id, state_name = :state_name, time_entered = :time_entered, last_updated = :last_updated, failing_count = :failing_count, response_count = :response_count", state)
	if err != nil {
		return err
	}

	return nil
}

func PutMemo(q sqlx.Ext, memo *ResultMemo) error {
	_, err := sqlx.NamedExec(q, "INSERT INTO check_state_memos (check_id, customer_id, bastion_id, failing_count, response_count, last_updated) VALUES (:check_id, :customer_id, :bastion_id, :failing_count, :response_count, :last_updated) ON CONFLICT (check_id) DO UPDATE SET failing_count = :failing_count, response_count = :response_count, last_updated = :last_updated", memo)
	if err != nil {
		return err
	}

	return nil
}

func GetMemo(q sqlx.Ext, checkId, bastionId string) (*ResultMemo, error) {
	memo := &ResultMemo{}
	err := sqlx.Select(q, memo, "SELECT * FROM check_state_memos WHERE check_id = ? AND bastion_id = ?", checkId, bastionId)
	if err != nil {
		return nil, err
	}

	return memo, nil
}
