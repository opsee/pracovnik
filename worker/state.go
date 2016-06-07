package worker

import (
	"fmt"
	"time"

	"github.com/opsee/basic/schema"
)

const (
	StateInvalid StateId = iota // ignore 0
	StateOK
	StateFailWait
	StatePassWait
	StateFail
	StateWarn
)

var (
	StateFnMap   = map[StateId]StateFn{}
	StateStrings = []string{
		"INVALID",
		"OK",
		"FAIL_WAIT",
		"PASS_WAIT",
		"FAIL",
		"WARN",
	}
)

func init() {
	StateFnMap[StateOK] = ok
	StateFnMap[StateFailWait] = failWait
	StateFnMap[StatePassWait] = passWait
	StateFnMap[StateFail] = fail
	StateFnMap[StateWarn] = warn
}

type StateId int

type StateFn func(*State) StateId

type ResultMemo struct {
	CheckId       string `json:"check_id"`
	CustomerId    string `json:"customer_id"`
	BastionId     string `json:"bastion_id"`
	NumFailing    int32  `json:"num_failing"`
	ResponseCount int    `json:"response_count"`

	// LastUpdate is result.Timestamp.Millis()
	LastUpdate int64 `json:"last_update"`
}

func ResultMemoFromCheckResult(result *schema.CheckResult) *ResultMemo {
	bastionId := result.BastionId
	if bastionId == "" {
		bastionId = result.CustomerId
	}

	return &ResultMemo{
		CheckId:       result.CheckId,
		CustomerId:    result.CustomerId,
		BastionId:     bastionId,
		NumFailing:    int32(result.FailingCount()),
		ResponseCount: len(result.Responses),
		LastUpdate:    result.Timestamp.Millis(),
	}
}

type State struct {
	CheckId         string                 `json:"check_id"`
	CustomerId      string                 `json:"customer_id"`
	Id              StateId                `json:"state_id"`
	State           string                 `json:"state_name"`
	TimeEntered     time.Time              `json:"time_entered"`
	LastUpdate      time.Time              `json:"last_update"`
	MinFailingCount int32                  `json:"min_failing_count"`
	MinFailingTime  time.Duration          `json:"min_failing_time"`
	NumFailing      int32                  `json:"num_failing"`
	Results         map[string]*ResultMemo // map[bastion_id]failing_count
}

// Transition is the transition function for the Check state machine. Given a
// proposed change to the current state (a new CheckResult object), update the
// state for the check associated with the result.
func (state *State) Transition(result *schema.CheckResult) error {
	// update failing count.
	var totalFails int32

	state.Results[result.BastionId] = ResultMemoFromCheckResult(result)
	for _, rm := range state.Results {
		totalFails += rm.NumFailing
	}
	state.NumFailing = totalFails

	state.LastUpdate = time.Now()

	sFn, ok := StateFnMap[state.Id]
	if !ok {
		return fmt.Errorf("Invalid state: %s", StateStrings[state.Id])
	}

	newSid := sFn(state)
	if newSid == StateInvalid {
		return fmt.Errorf("Invalid state transition.")
	}

	if newSid != state.Id {
		t := time.Now()
		state.TimeEntered = t
		state.LastUpdate = t
	}
	state.Id = newSid
	state.State = StateStrings[newSid]

	return nil
}

func ok(s *State) StateId {
	switch {
	case s.NumFailing == 0:
		return StateOK
	case 0 < s.NumFailing && s.NumFailing < s.MinFailingCount:
		return StateWarn
	case s.NumFailing >= s.MinFailingCount:
		return StateFailWait
	}

	return StateInvalid
}

func failWait(s *State) StateId {
	dt := s.LastUpdate.Sub(s.TimeEntered)

	switch {
	case s.NumFailing >= s.MinFailingCount && dt < s.MinFailingTime:
		return StateFailWait
	case s.NumFailing == 0:
		return StateOK
	case s.NumFailing >= s.MinFailingCount && dt > s.MinFailingTime:
		return StateFail
	case 0 < s.NumFailing && s.NumFailing < s.MinFailingCount:
		return StateWarn
	}

	return StateInvalid
}

func passWait(s *State) StateId {
	dt := s.LastUpdate.Sub(s.TimeEntered)

	switch {
	case s.NumFailing < s.MinFailingCount && dt < s.MinFailingTime:
		return StatePassWait
	case s.NumFailing >= s.MinFailingCount:
		return StateFail
	case 0 < s.NumFailing && s.NumFailing < s.MinFailingCount && dt > s.MinFailingTime:
		return StateWarn
	case s.NumFailing == 0 && dt > s.MinFailingTime:
		return StateOK
	}

	return StateInvalid
}

func fail(s *State) StateId {
	switch {
	case s.NumFailing >= s.MinFailingCount:
		return StateFail
	case s.NumFailing < s.MinFailingCount:
		return StatePassWait
	}

	return StateInvalid
}

func warn(s *State) StateId {
	switch {
	case 0 < s.NumFailing && s.NumFailing < s.MinFailingCount:
		return StateWarn
	case s.NumFailing == 0:
		return StateOK
	case s.NumFailing >= s.MinFailingCount:
		return StateFailWait
	}

	return StateInvalid
}
