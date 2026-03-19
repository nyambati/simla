package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/mocks"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newTestWatcher builds a Watcher with a mock scheduler and a short debounce
// so tests don't have to wait 500 ms.
func newTestWatcher(t *testing.T, cfg *config.Config, sched *mocks.MockSchedulerInterface, debounce time.Duration) *Watcher {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	return New(cfg, sched, logger.WithField("test", t.Name()), debounce)
}

// ── New / construction ────────────────────────────────────────────────────────

func TestNew_DefaultDebounce(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	cfg := &config.Config{Services: map[string]config.Service{}}

	w := New(cfg, sched, logrus.NewEntry(logrus.New()), 0)
	assert.Equal(t, defaultDebounce, w.debounce)
}

func TestNew_CustomDebounce(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	cfg := &config.Config{Services: map[string]config.Service{}}

	w := New(cfg, sched, logrus.NewEntry(logrus.New()), 200*time.Millisecond)
	assert.Equal(t, 200*time.Millisecond, w.debounce)
}

// ── Start with no watchable paths ─────────────────────────────────────────────

func TestStart_NoServices_WaitsForCancel(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)
	cfg := &config.Config{Services: map[string]config.Service{}}

	w := newTestWatcher(t, cfg, sched, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestStart_NonExistentPath_SkipsAndWaits(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	cfg := &config.Config{
		Services: map[string]config.Service{
			"svc": {CodePath: "/this/path/does/not/exist/ever"},
		},
	}

	w := newTestWatcher(t, cfg, sched, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Should not error — nonexistent paths are skipped with a warning.
	err := w.Start(ctx)
	assert.NoError(t, err)
}

// ── Hot reload on file change ──────────────────────────────────────────────────

func TestStart_FileChange_TriggersRestart(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	dir := t.TempDir()
	cfg := &config.Config{
		Services: map[string]config.Service{
			"payments": {CodePath: dir},
		},
	}

	stopped := make(chan string, 1)
	started := make(chan string, 1)

	sched.EXPECT().
		StopService(gomock.Any(), "payments").
		DoAndReturn(func(_ context.Context, name string) error {
			stopped <- name
			return nil
		})
	sched.EXPECT().
		StartService(gomock.Any(), "payments").
		DoAndReturn(func(_ context.Context, name string) error {
			started <- name
			return nil
		})

	w := newTestWatcher(t, cfg, sched, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	// Give the watcher time to start before writing.
	time.Sleep(100 * time.Millisecond)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))

	waitFor := func(ch chan string, label string) {
		t.Helper()
		select {
		case name := <-ch:
			assert.Equal(t, "payments", name)
		case <-time.After(3 * time.Second):
			t.Fatalf("expected %s for service 'payments', got none", label)
		}
	}

	waitFor(stopped, "StopService")
	waitFor(started, "StartService")

	cancel()
	<-done
}

func TestStart_Debounce_MultipleChanges_OneRestart(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	dir := t.TempDir()
	cfg := &config.Config{
		Services: map[string]config.Service{
			"svc": {CodePath: dir},
		},
	}

	restartCount := 0
	sched.EXPECT().StopService(gomock.Any(), "svc").DoAndReturn(func(_ context.Context, _ string) error {
		restartCount++
		return nil
	}).AnyTimes()
	sched.EXPECT().StartService(gomock.Any(), "svc").Return(nil).AnyTimes()

	w := newTestWatcher(t, cfg, sched, 150*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	time.Sleep(80 * time.Millisecond)

	// Write 5 times in rapid succession — debounce should collapse to 1 restart.
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte("change"), 0644)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait longer than debounce to let timer fire.
	time.Sleep(400 * time.Millisecond)

	cancel()
	<-done

	assert.Equal(t, 1, restartCount, "debounce should collapse rapid changes to a single restart")
}

// ── Independent service isolation ─────────────────────────────────────────────

func TestStart_TwoServices_ChangeOnlyRestartsTouchedService(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	dirA := t.TempDir()
	dirB := t.TempDir()

	cfg := &config.Config{
		Services: map[string]config.Service{
			"serviceA": {CodePath: dirA},
			"serviceB": {CodePath: dirB},
		},
	}

	restartedA := make(chan struct{}, 1)
	// serviceA gets restarted.
	sched.EXPECT().StopService(gomock.Any(), "serviceA").DoAndReturn(func(_ context.Context, _ string) error {
		restartedA <- struct{}{}
		return nil
	})
	sched.EXPECT().StartService(gomock.Any(), "serviceA").Return(nil)
	// serviceB must NOT be touched.
	sched.EXPECT().StopService(gomock.Any(), "serviceB").Times(0)

	w := newTestWatcher(t, cfg, sched, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(dirA, "handler.go"), []byte("package main"), 0644))

	select {
	case <-restartedA:
	case <-time.After(3 * time.Second):
		t.Fatal("serviceA was not restarted")
	}

	// Extra wait to give any unwanted serviceB restart a chance to manifest.
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done
}

// ── restart error handling ─────────────────────────────────────────────────────

func TestRestart_StopServiceError_DoesNotCallStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().StopService(gomock.Any(), "svc").Return(errors.New("stop failed"))
	// StartService must NOT be called if StopService fails.
	sched.EXPECT().StartService(gomock.Any(), gomock.Any()).Times(0)

	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	w := &Watcher{
		config:    &config.Config{},
		scheduler: sched,
		logger:    logger.WithField("test", t.Name()),
		debounce:  defaultDebounce,
	}
	// Call restart directly.
	w.restart(context.Background(), "svc")
}

func TestRestart_StartServiceError_IsLogged(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().StopService(gomock.Any(), "svc").Return(nil)
	sched.EXPECT().StartService(gomock.Any(), "svc").Return(errors.New("start failed"))

	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	w := &Watcher{
		config:    &config.Config{},
		scheduler: sched,
		logger:    logger.WithField("test", t.Name()),
		debounce:  defaultDebounce,
	}
	// Must not panic; error is logged.
	assert.NotPanics(t, func() {
		w.restart(context.Background(), "svc")
	})
}

// ── Context value propagation ─────────────────────────────────────────────────

func TestRestart_InjectsServiceNameIntoContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	sched := mocks.NewMockSchedulerInterface(ctrl)

	sched.EXPECT().StopService(gomock.Any(), "payments").Return(nil)
	sched.EXPECT().
		StartService(gomock.Any(), "payments").
		DoAndReturn(func(ctx context.Context, name string) error {
			val, ok := ctx.Value("service").(string)
			assert.True(t, ok, "context should contain service key")
			assert.Equal(t, "payments", val)
			return nil
		})

	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	w := &Watcher{
		config:    &config.Config{},
		scheduler: sched,
		logger:    logger.WithField("test", t.Name()),
		debounce:  defaultDebounce,
	}
	w.restart(context.Background(), "payments")
}
