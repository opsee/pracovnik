package worker

import (
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestTransactionIsolation(t *testing.T) {
	db, err := sqlx.Open("postgres", viper.GetString("postgres_conn"))
	assert.Nil(t, err)

	// First we make sure we have memos and state.
	state := &State{
		CheckId:     "check-id",
		CustomerId:  "11111111-1111-1111-1111-111111111111",
		Id:          StateOK,
		State:       StateOK.String(),
		TimeEntered: time.Now(),
		LastUpdated: time.Now(),
	}
	err = PutState(db, state)
	assert.Nil(t, err)

	err = PutMemo(db, &ResultMemo{
		BastionId:     "61f25e94-4f6e-11e5-a99f-4771161a3518",
		CustomerId:    "11111111-1111-1111-1111-111111111111",
		CheckId:       "check-id",
		FailingCount:  0,
		ResponseCount: 2,
	})
	assert.Nil(t, err)

	err = PutMemo(db, &ResultMemo{
		BastionId:     "61f25e94-4f6e-11e5-a99f-4771161a3517",
		CustomerId:    "11111111-1111-1111-1111-111111111111",
		CheckId:       "check-id",
		FailingCount:  0,
		ResponseCount: 2,
	})
	assert.Nil(t, err)

	// There is some non-determinism here... we don't know which goroutine is
	// going to get the row lock first. Huzzah, testing actual concurrency.
	// So, whatever happens, the operation we're doing does have to have a
	// predictable output.

	fakeworker := func(wg *sync.WaitGroup, bastionId string) {
		defer wg.Done()

		tx, err := db.Beginx()
		assert.Nil(t, err)

		state, err = GetAndLockState(tx, "11111111-1111-1111-1111-111111111111", "check-id")
		assert.Nil(t, err)
		assert.NotNil(t, state)

		err = PutMemo(tx, &ResultMemo{
			BastionId:     bastionId,
			CustomerId:    "11111111-1111-1111-1111-111111111111",
			CheckId:       "check-id",
			FailingCount:  2,
			ResponseCount: 2,
		})
		assert.Nil(t, err)

		assert.Nil(t, UpdateState(tx, state))
		assert.Nil(t, state.Transition(nil))
		assert.Nil(t, PutState(tx, state))
		tx.Commit()
	}
	wg := &sync.WaitGroup{}
	// no guarantee which one locks first, but the outcome should be the same.
	wg.Add(2)
	go fakeworker(wg, "61f25e94-4f6e-11e5-a99f-4771161a3517")
	go fakeworker(wg, "61f25e94-4f6e-11e5-a99f-4771161a3518")
	wg.Wait()

	tx, err := db.Beginx()
	assert.Nil(t, err)
	defer tx.Rollback()
	state, err = GetAndLockState(tx, "11111111-1111-1111-1111-111111111111", "check-id")
	assert.Nil(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, int32(4), state.FailingCount)
	assert.Equal(t, int32(4), state.ResponseCount)

}
