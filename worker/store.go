package worker

import (
	"time"

	"github.com/opsee/basic/schema"
)

var (
	store *StateStore
)

type StateStore struct {
}

func GetCheck(customerId, checkId string) (*schema.Check, error) {
	return store.GetCheck(customerId, checkId)
}

func GetState(customerId, checkId string) (*State, error) {
	return store.GetState(customerId, checkId)
}

func PutState(state *State) error {
	return store.PutState(state)
}

func (store *StateStore) GetCheck(customerId, checkId string) (*schema.Check, error) {
	return nil, nil
}

// GetState creates a State object populated by the check's settings and
// by the current state if it exists. If it the state is unknown, then it
// assumes a present state of OK.
func (store *StateStore) GetState(customerId, checkId string) (*State, error) {
	// Get the check so that we can get MinFailingCount and MinFailingTime
	// Return an error if the check doesn't exist
	check, err := store.GetCheck(customerId, checkId)
	if err != nil {
		return nil, err
	}

	state, err := store.GetState(customerId, checkId)
	if err != nil {
		return nil, err
	}

	if state != nil {
		return state, nil
	}

	return &State{
		CheckId:         checkId,
		CustomerId:      customerId,
		Id:              StateOK,
		State:           StateStrings[StateOK],
		TimeEntered:     time.Now(),
		LastUpdate:      time.Now(),
		MinFailingCount: check.MinFailingCount,
		MinFailingTime:  time.Duration(check.MinFailingTime) * time.Second,
		NumFailing:      0,
	}, nil
}

func (store *StateStore) PutState(state *State) error {
	return nil
}
