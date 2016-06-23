package worker

import (
	"testing"
	"time"

	"github.com/opsee/basic/schema"
	opsee_types "github.com/opsee/protobuf/opseeproto/types"
	"github.com/stretchr/testify/assert"
)

func testMockState(sid StateId, f, n int, t, te time.Time, tw time.Duration) *State {
	return &State{
		FailingCount:    int32(n),
		MinFailingCount: int32(f),
		TimeEntered:     te,
		LastUpdated:     t,
		Id:              sid,
		State:           sid.String(),
		MinFailingTime:  tw,
	}
}

func testMockResult(responseCount, failingCount int) *schema.CheckResult {
	ts := &opsee_types.Timestamp{}
	ts.Scan(time.Now())
	r := &schema.CheckResult{
		CheckId:    "check-id",
		CustomerId: "11111111-1111-1111-1111-111111111111",
		BastionId:  "61f25e94-4f6e-11e5-a99f-4771161a3518",
		Responses:  make([]*schema.CheckResponse, responseCount),
		Timestamp:  ts,
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
	s.FailingCount = 0

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestOkToWarn(t *testing.T) {
	s := testMockState(StateOK, 2, 0, time.Now(), time.Now(), 0)
	r := testMockResult(2, 1)
	s.FailingCount = 1

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestOkToFailWait(t *testing.T) {
	s := testMockState(StateOK, 2, 0, time.Now(), time.Now(), 0)
	r := testMockResult(2, 2)
	s.FailingCount = 2

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL_WAIT", s.State)
}

func TestFailWaitToFailWait(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now(), 30*time.Second)
	r := testMockResult(2, 2)
	s.FailingCount = 2

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL_WAIT", s.State)
}

func TestFailWaitToOk(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now(), 0)
	r := testMockResult(2, 0)
	s.FailingCount = 0

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestFailWaitToFail(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 2)
	s.FailingCount = 2

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL", s.State)
}

func TestFailWaitToWarn(t *testing.T) {
	s := testMockState(StateFailWait, 2, 2, time.Now(), time.Now(), 0)
	r := testMockResult(2, 1)
	s.FailingCount = 1

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestPassWaitToPassWait(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now(), 30*time.Second)
	r := testMockResult(2, 1)
	s.FailingCount = 1

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "PASS_WAIT", s.State)
}

func TestPassWaitToFail(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now(), 30*time.Second)
	r := testMockResult(2, 2)
	s.FailingCount = 2

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL", s.State)
}

func TestPassWaitToWarn(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 1)
	s.FailingCount = 1

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestPassWaitToOk(t *testing.T) {
	s := testMockState(StatePassWait, 2, 1, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 0)
	s.FailingCount = 0

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestFailToFail(t *testing.T) {
	s := testMockState(StateFail, 2, 2, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 2)
	s.FailingCount = 2

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL", s.State)
}

func TestFailToPassWait(t *testing.T) {
	s := testMockState(StateFail, 2, 2, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 1)
	s.FailingCount = 1

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "PASS_WAIT", s.State)
}

func TestWarnToWarn(t *testing.T) {
	s := testMockState(StateWarn, 2, 1, time.Now(), time.Now().Add(-1*time.Minute), 30*time.Second)
	r := testMockResult(2, 1)
	s.FailingCount = 1

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "WARN", s.State)
}

func TestWarnToOk(t *testing.T) {
	s := testMockState(StateWarn, 2, 1, time.Now(), time.Now(), 0)
	r := testMockResult(2, 0)
	s.FailingCount = 0

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "OK", s.State)
}

func TestWarnToFailWait(t *testing.T) {
	s := testMockState(StateWarn, 2, 1, time.Now(), time.Now(), 0)
	r := testMockResult(2, 2)
	s.FailingCount = 2

	err := s.Transition(r)
	assert.Nil(t, err)
	assert.Equal(t, "FAIL_WAIT", s.State)
}
