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

type State struct {
	CheckId         string        `json:"check_id"`
	Id              StateId       `json:"-"`
	State           string        `json:"state"`
	TimeEntered     time.Time     `json:"time_entered"`
	LastUpdate      time.Time     `json:"last_update"`
	MinFailingCount int           `json:"min_failing_count"`
	MinFailingTime  time.Duration `json:"min_failing_time"`
	NumFailing      int           `json:"num_failing"`

	fails map[string]int // map[bastion_id]failing_count
}

func transition(state *State, result *schema.CheckResult) (*State, error) {
	// update failing count.
	var totalFails int

	state.fails[result.BastionId] = result.FailingCount()
	for _, c := range state.fails {
		totalFails += c
	}
	state.NumFailing = totalFails

	if time.Now().After(state.LastUpdate) {
		state.LastUpdate = time.Now()
	}

	sFn, ok := StateFnMap[state.Id]
	if !ok {
		return nil, fmt.Errorf("Invalid state: %s", StateStrings[state.Id])
	}

	newSid := sFn(state)
	if newSid == StateInvalid {
		return nil, fmt.Errorf("Invalid state transition.")
	}

	if newSid != state.Id {
		t := time.Now()
		state.TimeEntered = t
		state.LastUpdate = t
	}
	state.Id = newSid
	state.State = StateStrings[newSid]

	return state, nil
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
