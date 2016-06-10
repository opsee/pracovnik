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

	transitionHooks = map[StateId][]TransitionHook{}
)

func init() {
	StateFnMap[StateOK] = ok
	StateFnMap[StateFailWait] = failWait
	StateFnMap[StatePassWait] = passWait
	StateFnMap[StateFail] = fail
	StateFnMap[StateWarn] = warn
}

type StateId int

type StateFn func(state *State) StateId

type TransitionHook func(newStateId StateId, state *State)

type ResultMemo struct {
	CheckId       string `json:"check_id" db:"check_id"`
	CustomerId    string `json:"customer_id" db:"customer_id"`
	BastionId     string `json:"bastion_id" db:"bastion_id"`
	FailingCount  int32  `json:"failing_count" db:"failing_count"`
	ResponseCount int    `json:"response_count" db:"response_count"`

	// LastUpdated is result.Timestamp.Millis()
	LastUpdated time.Time `json:"last_updated" db:"last_updated"`
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
		FailingCount:  int32(result.FailingCount()),
		ResponseCount: len(result.Responses),
		LastUpdated:   time.Unix(result.Timestamp.Seconds, int64(result.Timestamp.Nanos)),
	}
}

type State struct {
	CheckId         string                 `json:"check_id" db:"check_id"`
	CustomerId      string                 `json:"customer_id" db:"customer_id"`
	Id              StateId                `json:"state_id" db:"state_id"`
	State           string                 `json:"state_name" db:"state_name"`
	TimeEntered     time.Time              `json:"time_entered" db:"time_entered"`
	LastUpdated     time.Time              `json:"last_updated" db:"last_updated"`
	MinFailingCount int32                  `json:"min_failing_count" db:"min_failing_count"`
	MinFailingTime  time.Duration          `json:"min_failing_time" db:"min_failing_time"`
	FailingCount    int32                  `json:"failing_count" db:"failing_count"`
	ResponseCount   int32                  `json:"response_count" db:"response_count"`
	Results         map[string]*ResultMemo `json:"-"` // map[bastion_id]failing_count
}

func AddHook(hook TransitionHook) {
	for state, _ := range transitionHooks {
		AddStateHook(state, hook)
	}
}

func AddStateHook(id StateId, hook TransitionHook) {
	_, ok := transitionHooks[id]
	if !ok {
		transitionHooks[id] = []TransitionHook{}
	}

	transitionHooks[id] = append(transitionHooks[id], hook)
}

func callHooks(id StateId, state *State) {
	hooks, ok := transitionHooks[id]
	if ok {
		for _, hook := range hooks {
			hook(id, state)
		}
	}
}

func (state *State) TimeInState() time.Duration {
	return state.LastUpdated.Sub(state.TimeEntered)
}

// Transition is the transition function for the Check state machine. Given a
// proposed change to the current state (a new CheckResult object), update the
// state for the check associated with the result.
func (state *State) Transition(result *schema.CheckResult) error {
	// update failing count.
	var (
		totalFails    int32
		responseCount int32
	)

	state.Results[result.BastionId] = ResultMemoFromCheckResult(result)
	for _, memo := range state.Results {
		responseCount += int32(memo.ResponseCount)
		totalFails += memo.FailingCount
	}
	state.FailingCount = totalFails
	state.ResponseCount = responseCount

	state.LastUpdated = time.Now()

	sFn, ok := StateFnMap[state.Id]
	if !ok {
		return fmt.Errorf("Invalid state: %s", StateStrings[state.Id])
	}

	newSid := sFn(state)
	if newSid == StateInvalid {
		return fmt.Errorf("Invalid state transition.")
	}

	if newSid != state.Id {
		// hooks should be called on the state _before_ it has been modified.
		callHooks(newSid, state)
		t := time.Now()
		state.TimeEntered = t
		state.LastUpdated = t
	}
	state.Id = newSid
	state.State = StateStrings[newSid]

	return nil
}

func ok(s *State) StateId {
	switch {
	case s.FailingCount == 0:
		return StateOK
	case 0 < s.FailingCount && s.FailingCount < s.MinFailingCount:
		return StateWarn
	case s.FailingCount >= s.MinFailingCount:
		return StateFailWait
	}

	return StateInvalid
}

func failWait(s *State) StateId {
	switch {
	case s.FailingCount >= s.MinFailingCount && s.TimeInState() < s.MinFailingTime:
		return StateFailWait
	case s.FailingCount == 0:
		return StateOK
	case s.FailingCount >= s.MinFailingCount && s.TimeInState() > s.MinFailingTime:
		return StateFail
	case 0 < s.FailingCount && s.FailingCount < s.MinFailingCount:
		return StateWarn
	}

	return StateInvalid
}

func passWait(s *State) StateId {
	switch {
	case s.FailingCount < s.MinFailingCount && s.TimeInState() < s.MinFailingTime:
		return StatePassWait
	case s.FailingCount >= s.MinFailingCount:
		return StateFail
	case 0 < s.FailingCount && s.FailingCount < s.MinFailingCount && s.TimeInState() > s.MinFailingTime:
		return StateWarn
	case s.FailingCount == 0 && s.TimeInState() > s.MinFailingTime:
		return StateOK
	}

	return StateInvalid
}

func fail(s *State) StateId {
	switch {
	case s.FailingCount >= s.MinFailingCount:
		return StateFail
	case s.FailingCount < s.MinFailingCount:
		return StatePassWait
	}

	return StateInvalid
}

func warn(s *State) StateId {
	switch {
	case 0 < s.FailingCount && s.FailingCount < s.MinFailingCount:
		return StateWarn
	case s.FailingCount == 0:
		return StateOK
	case s.FailingCount >= s.MinFailingCount:
		return StateFailWait
	}

	return StateInvalid
}
