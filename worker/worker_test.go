package worker

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/opsee/basic/schema"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type fakeStore struct {
	fail bool
}

func (s *fakeStore) PutResult(result *schema.CheckResult) error {
	if s.fail {
		return errors.New("")
	}

	return nil
}

func (s *fakeStore) GetResultsByCheckId(checkId string) (map[string]*schema.CheckResult, error) {
	if s.fail {
		return nil, errors.New("")
	}

	return nil, nil
}

func TestPutResultFailure(t *testing.T) {
	db, err := sqlx.Open("postgres", viper.GetString("postgres_conn"))
	assert.Nil(t, err)
	dynamo := &fakeStore{true}
	result := testMockResult(2, 0)

	wrkr := NewCheckWorker(db, dynamo, result)
	_, err = wrkr.Execute()
	assert.NotNil(t, err)
}

func TestExistingState(t *testing.T) {
	db, err := sqlx.Open("postgres", viper.GetString("postgres_conn"))
	assert.Nil(t, err)
	dynamo := &fakeStore{false}
	result := testMockResult(2, 0)

	state := &State{
		CheckId:     "check-id",
		CustomerId:  "11111111-1111-1111-1111-111111111111",
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

func testSetupFixtures() {
	db, err := sqlx.Open("postgres", viper.GetString("postgres_conn"))
	if err != nil {
		panic(err)
	}
	check := &schema.Check{
		Id:               "check-id",
		CustomerId:       "11111111-1111-1111-1111-111111111111",
		ExecutionGroupId: "11111111-1111-1111-1111-111111111111",
		Name:             "check",
		MinFailingCount:  1,
		MinFailingTime:   90,
	}
	_, err = sqlx.NamedExec(db, "INSERT INTO checks (id, min_failing_count, min_failing_time, customer_id, execution_group_id, name, target_type, target_id) VALUES (:id, :min_failing_count, :min_failing_time, :customer_id, :execution_group_id, :name, 'target-id', 'target-type')", check)
	if err != nil {
		panic(err)
	}
}

func TestMain(m *testing.M) {
	viper.SetEnvPrefix("pracovnik")
	viper.AutomaticEnv()

	testSetupFixtures()

	os.Exit(m.Run())
}
