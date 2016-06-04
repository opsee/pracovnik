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
	checkId string        `json:"check_id"`
	id      stateId       `json:"state"`
	te      time.Time     `json:"time_entered"`
	t       time.Time     `json:"last_update"`
	f       int           `json:"min_failing_count"`
	tw      time.Duration `json:"min_failing_time"`
	n       int           `json:"num_failing"`

	fails map[string]int // map[bastion_id]failing_count
}

func transition(state *state, result *schema.CheckResult) (*state, error) {
	// update failing count.
	var totalFails int

	state.fails[result.BastionId] = result.FailingCount()
	for _, c := range state.fails {
		totalFails += c
	}
	state.n = totalFails

	if time.Now().After(state.t) {
		state.t = time.Now()
	}

	sFn, ok := stateFnMap[state.id]
	if !ok {
		return nil, fmt.Errorf("Invalid state: %s", stateStrings[state.id])
	}

	newSid := sFn(state)
	if newSid == StateInvalid {
		return nil, fmt.Errorf("Invalid state transition.")
	}

	if newSid != state.id {
		t := time.Now()
		state.te = t
		state.t = t
	}
	state.id = newSid

	return state, nil
}

func ok(s *state) stateId {
	switch {
	case s.n == 0:
		return StateOK
	case 0 < s.n && s.n < s.f:
		return StateWarn
	case s.n >= s.f:
		return StateFailWait
	}

	return StateInvalid
}

func failWait(s *state) stateId {
	dt := s.t.Sub(s.te)

	switch {
	case s.n >= s.f && dt < s.tw:
		return StateFailWait
	case s.n == 0:
		return StateOK
	case s.n >= s.f && dt > s.tw:
		return StateFail
	case 0 < s.n && s.n < s.f:
		return StateWarn
	}

	return StateInvalid
}

func passWait(s *state) stateId {
	dt := s.t.Sub(s.te)

	switch {
	case s.n < s.f && dt < s.tw:
		return StatePassWait
	case s.n >= s.f:
		return StateFail
	case 0 < s.n && s.n < s.f && dt > s.tw:
		return StateWarn
	case s.n == 0 && dt > s.tw:
		return StateOK
	}

	return StateInvalid
}

func fail(s *state) stateId {
	switch {
	case s.n >= s.f:
		return StateFail
	case s.n < s.f:
		return StatePassWait
	}

	return StateInvalid
}

func warn(s *state) stateId {
	switch {
	case 0 < s.n && s.n < s.f:
		return StateWarn
	case s.n == 0:
		return StateOK
	case s.n >= s.f:
		return StateFailWait
	}

	return StateInvalid
}
