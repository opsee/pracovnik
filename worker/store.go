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
	err := sqlx.Get(q, state, "SELECT cs.id, cs.customer_id, cs.check_id, cs.state, cs.time_entered, cs.last_update, c.min_failing_count, c.min_failing_time, cs.num_failing FROM check_state AS cs JOIN checks AS c ON (c.id=cs.check_id) WHERE cs.customer_id=? AND cs.id=?", customerId, checkId)
	if err != sql.ErrNoRows {
		return nil, err
	}

	if err == sql.ErrNoRows {
		// Get the check so that we can get MinFailingCount and MinFailingTime
		// Return an error if the check doesn't exist
		// check, err := store.GetCheck(customerId, checkId)
		check := &schema.Check{}
		err := sqlx.Get(q, check, "SELECT id, customer_id, min_failing_count, min_failing_time FROM checks WHERE customer_id=? AND check_id=?", customerId, checkId)
		if err != nil {
			return nil, err
		}

		state = &State{
			CheckId:         checkId,
			CustomerId:      customerId,
			Id:              StateOK,
			State:           StateStrings[StateOK],
			TimeEntered:     time.Now(),
			LastUpdate:      time.Now(),
			MinFailingCount: check.MinFailingCount,
			MinFailingTime:  time.Duration(check.MinFailingTime) * time.Second,
			NumFailing:      0,
			Results:         map[string]*ResultMemo{},
		}
	}

	memos := []*ResultMemo{}
	err = sqlx.Select(q, memos, "SELECT * FROM check_state_memos WHERE customer_id=? AND check_id=?", customerId, checkId)
	if err != sql.ErrNoRows {
		return nil, err
	}

	for _, memo := range memos {
		state.Results[memo.BastionId] = memo
	}

	return state, nil
}

func PutState(q sqlx.Ext, state *State) error {
	_, err := sqlx.NamedExec(q, "INSERT INTO check_state (check_id, customer_id, state_id, state_name, time_entered, last_update) VALUES (:check_id, :customer_id, :state_id, :state_name, :time_entered, :last_update, :num_failing) ON CONFLICT UPDATE", state)
	return err
}
