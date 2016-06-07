package worker

import (
	"fmt"
	"time"

	"github.com/opsee/basic/schema"
)

const (
	StateInvalid stateId = iota // ignore 0
	StateOK
	StateFailWait
	StatePassWait
	StateFail
	StateWarn
)

var (
	stateFnMap   = map[stateId]stateFn{}
	stateStrings = []string{
		"INVALID",
		"OK",
		"FAIL_WAIT",
		"PASS_WAIT",
		"FAIL",
		"WARN",
	}
)

func init() {
	stateFnMap[StateOK] = ok
	stateFnMap[StateFailWait] = failWait
	stateFnMap[StatePassWait] = passWait
	stateFnMap[StateFail] = fail
	stateFnMap[StateWarn] = warn
}

type stateId int

type stateFn func(*state) stateId

type state struct {
	CheckId         string        `json:"check_id"`
	Id              stateId       `json:"-"`
	State           string        `json:"state"`
	TimeEntered     time.Time     `json:"time_entered"`
	LastUpdate      time.Time     `json:"last_update"`
	MinFailingCount int           `json:"min_failing_count"`
	MinFailingTime  time.Duration `json:"min_failing_time"`
	NumFailing      int           `json:"num_failing"`

	fails map[string]int // map[bastion_id]failing_count
}

func transition(state *state, result *schema.CheckResult) (*state, error) {
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

	sFn, ok := stateFnMap[state.Id]
	if !ok {
		return nil, fmt.Errorf("Invalid state: %s", stateStrings[state.Id])
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
	state.State = stateStrings[newSid]

	return state, nil
}

func ok(s *state) stateId {
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

func failWait(s *state) stateId {
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

func passWait(s *state) stateId {
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

func fail(s *state) stateId {
	switch {
	case s.NumFailing >= s.MinFailingCount:
		return StateFail
	case s.NumFailing < s.MinFailingCount:
		return StatePassWait
	}

	return StateInvalid
}

func warn(s *state) stateId {
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
