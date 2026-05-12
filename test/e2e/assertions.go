//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

type timeDuration = time.Duration

func now() time.Time { return time.Now() }

func sleep(ms int) { time.Sleep(time.Duration(ms) * time.Millisecond) }

func AssertEventually(t *testing.T, timeout time.Duration, ok func() bool, detail func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	if detail != nil {
		t.Fatal(detail())
	}
	t.Fatal("condition was not met before timeout")
}

func AssertSession(t *testing.T, env *TestEnv, title string, check func(db.Session) error) db.Session {
	t.Helper()
	conn, err := db.Open(env.DBPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	s, err := db.GetSessionByTitle(conn, title)
	if err != nil {
		t.Fatalf("get session %q: %v", title, err)
	}
	if check != nil {
		if err := check(s); err != nil {
			t.Fatalf("session %q assertion failed: %v\nsession: %+v", title, err, s)
		}
	}
	return s
}

func expectEqual[T comparable](field string, got, want T) error {
	if got != want {
		return fmt.Errorf("%s=%v, want %v", field, got, want)
	}
	return nil
}
