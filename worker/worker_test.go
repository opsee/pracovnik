package worker

import (
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/opsee/basic/schema"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type fakeDynamo struct {
	fail bool
}

func (s *fakeDynamo) PutResult(result *schema.CheckResult) error {
	if s.fail {
		return errors.New("")
	}

	return nil
}

func (s *fakeDynamo) GetResultsByCheckId(checkId string) (map[string]*schema.CheckResult, error) {
	if s.fail {
		return nil, errors.New("")
	}

	return nil, nil
}

func TestPutResultFailure(t *testing.T) {
	db, err := sqlx.Open("postgres", viper.GetString("postgres_conn"))
	assert.Nil(t, err)
	dynamo := &fakeDynamo{true}
	result := testMockResult(2, 0)

	wrkr := NewCheckWorker(db, dynamo, result)
	_, err = wrkr.Execute()
	assert.NotNil(t, err)
}

func TestExistingState(t *testing.T) {
	db, err := sqlx.Open("postgres", viper.GetString("postgres_conn"))
	assert.Nil(t, err)
	dynamo := &fakeDynamo{false}
	result := testMockResult(2, 0)

	state := &State{
		CheckId:     "check-id",
		CustomerId:  "customer-id",
		Id:          StateOK,
		State:       "OK",
		TimeEntered: time.Now(),
		LastUpdated: time.Now(),
	}

	err = PutState(db, state)
	assert.Nil(t, err)

	wrkr := NewCheckWorker(db, dynamo, result)
	_, err = wrkr.Execute()
	assert.Nil(t, err)

}

func TestMain(m *testing.M) {
	viper.SetEnvPrefix("pracovnik")
	viper.AutomaticEnv()
}
