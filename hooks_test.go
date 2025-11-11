package crud

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubContext struct {
	ctx context.Context
}

func newStubContext() *stubContext {
	return &stubContext{ctx: context.Background()}
}

func (s *stubContext) UserContext() context.Context {
	return s.ctx
}

func (s *stubContext) Params(string, ...string) string {
	return ""
}

func (s *stubContext) BodyParser(out any) error {
	return nil
}

func (s *stubContext) Query(string, ...string) string {
	return ""
}

func (s *stubContext) QueryInt(string, ...int) int {
	return 0
}

func (s *stubContext) Queries() map[string]string {
	return map[string]string{}
}

func (s *stubContext) Body() []byte {
	return nil
}

func (s *stubContext) Status(int) Response {
	return s
}

func (s *stubContext) JSON(any, ...string) error {
	return nil
}

func (s *stubContext) SendStatus(int) error {
	return nil
}

func TestHookFromContextAdaptsLegacyFunc(t *testing.T) {
	legacyCtx := newStubContext()
	var receivedCtx Context
	var receivedUser *TestUser

	hook := HookFromContext(func(ctx Context, user *TestUser) error {
		receivedCtx = ctx
		receivedUser = user
		return nil
	})

	user := &TestUser{Name: "legacy"}
	err := hook(HookContext{Context: legacyCtx}, user)
	require.NoError(t, err)

	assert.Equal(t, legacyCtx, receivedCtx)
	assert.Equal(t, user, receivedUser)
}

func TestHookBatchFromContextAdaptsLegacyFunc(t *testing.T) {
	legacyCtx := newStubContext()
	var receivedCtx Context
	var received []*TestUser

	hook := HookBatchFromContext(func(ctx Context, users []*TestUser) error {
		receivedCtx = ctx
		received = users
		return nil
	})

	users := []*TestUser{{Name: "a"}, {Name: "b"}}
	err := hook(HookContext{Context: legacyCtx}, users)
	require.NoError(t, err)

	assert.Equal(t, legacyCtx, receivedCtx)
	assert.Equal(t, users, received)
}
