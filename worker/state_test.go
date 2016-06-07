package worker

import (
	"testing"
	"time"

	"github.com/opsee/basic/schema"
	"github.com/stretchr/testify/assert"
)

func testMockState(sid StateId, f, n int, t, te time.Time, tw time.Duration) *State {
	return &State{
		NumFailing:      n,
		MinFailingCount: f,
		TimeEntered:     te,
		LastUpdate:      t,
		Id:              sid,
		State:           StateStrings[sid],
		MinFailingTime:  tw,
		fails: map[string]int{
			"bastion-id": n,
		},
	}
}

func testMockResult(responseCount, failingCount int) *schema.CheckResult {
	r := &schema.CheckResult{
		BastionId: "bastion-id",
		Responses: make([]*schema.CheckResponse, responseCount),
	}

	for i := 0; i < responseCount; i++ {
		r.Responses[i] = &schema.CheckResponse{}
		if i >= failingCount {
			r.Responses[i].Passing = true
		}
	}

	return r
}

func TestOkToOk(t *testing.T) {
	s := testMockState(StateOK, 2, 0, time.Now(), time.Now(), 0)
	r := testMockResult(2, 0)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestOkToWarn(t *testing.T) {
	s := testMockState(StateOK, 2, 0, time.Now(), time.Now(), 0)
	r := testMockResult(2, 1)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestOkToFailWait(t *testing.T) {
	s := testMockState(StateOK, 2, 0, time.Now(), time.Now(), 0)
	r := testMockResult(2, 2)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL_WAIT", s.State)
}

func TestFailWaitToFailWait(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now(), 30*time.Second)
	r := testMockResult(2, 2)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL_WAIT", s.State)
}

func TestFailWaitToOk(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now(), 0)
	r := testMockResult(2, 0)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestFailWaitToFail(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 2)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL", s.State)
}

func TestFailWaitToWarn(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now(), 0)
	r := testMockResult(2, 1)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestPassWaitToPassWait(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now(), 30*time.Second)
	r := testMockResult(2, 1)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "PASS_WAIT", s.State)
}

func TestPassWaitToFail(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now(), 30*time.Second)
	r := testMockResult(2, 2)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL", s.State)
}

func TestPassWaitToWarn(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 1)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestPassWaitToOk(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 0)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestFailToFail(t *testing.T) {
	s := testMockState(StateFail, 2, 2, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 2)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL", s.State)
}

func TestFailToPassWait(t *testing.T) {
	s := testMockState(StateFail, 2, 2, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 1)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "PASS_WAIT", s.State)
}

func TestWarnToWarn(t *testing.T) {
	s := testMockState(StateWarn, 2, 1, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 1)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestWarnToOk(t *testing.T) {
	s := testMockState(StateWarn, 2, 1, time.Now(), time.Now(), 0)
	r := testMockResult(2, 0)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestWarnToFailWait(t *testing.T) {
	s := testMockState(StateWarn, 2, 1, time.Now(), time.Now(), 0)
	r := testMockResult(2, 2)

	s, err := transition(s, r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL_WAIT", s.State)
}
