// Package watcher provides file-system watch-based hot reload for Lambda
// services. When a file inside a service's codePath changes, the watcher
// debounces the event and triggers a container restart via the scheduler.
package watcher

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nyambati/simla/internal/config"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/sirupsen/logrus"
)

const defaultDebounce = 500 * time.Millisecond

// Watcher monitors each service's codePath and restarts the container when
// files change.
type Watcher struct {
	config    *config.Config
	scheduler scheduler.SchedulerInterface
	logger    *logrus.Entry
	debounce  time.Duration
}

// New creates a Watcher. debounce controls how long to wait after the last
// file event before triggering a restart; pass 0 to use the default (500ms).
func New(cfg *config.Config, sched scheduler.SchedulerInterface, logger *logrus.Entry, debounce time.Duration) *Watcher {
	if debounce == 0 {
		debounce = defaultDebounce
	}
	return &Watcher{
		config:    cfg,
		scheduler: sched,
		logger:    logger.WithField("component", "watcher"),
		debounce:  debounce,
	}
}

// Start begins watching all configured service codePaths. It blocks until ctx
// is cancelled. Each service path is watched independently; a change in one
// service does not restart others.
func (w *Watcher) Start(ctx context.Context) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsw.Close()

	// Map from absolute codePath → service name so we can identify which
	// service to restart when an event fires.
	pathToService := make(map[string]string)

	for name, svc := range w.config.Services {
		abs, err := filepath.Abs(svc.CodePath)
		if err != nil {
			w.logger.WithError(err).Warnf("skipping watch for service %s: cannot resolve path %s", name, svc.CodePath)
			continue
		}
		if err := fsw.Add(abs); err != nil {
			w.logger.WithError(err).Warnf("skipping watch for service %s: cannot watch path %s", name, abs)
			continue
		}
		pathToService[abs] = name
		w.logger.WithFields(logrus.Fields{"service": name, "path": abs}).Info("watching code path for changes")
	}

	if len(pathToService) == 0 {
		w.logger.Warn("no service paths could be watched; hot reload disabled")
		<-ctx.Done()
		return nil
	}

	// Per-service debounce timers. Protected by a mutex because fsnotify
	// delivers events on its own goroutine.
	type timerEntry struct {
		timer *time.Timer
		mu    sync.Mutex
	}
	timers := make(map[string]*timerEntry, len(pathToService))
	for _, name := range pathToService {
		timers[name] = &timerEntry{}
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			// Only react to write/create/rename events (not chmod).
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}

			dir := filepath.Dir(event.Name)
			serviceName, found := pathToService[dir]
			if !found {
				// The event is for the watched dir itself (e.g. file inside it).
				// Try matching the dir directly (fsnotify reports the full path).
				serviceName, found = pathToService[event.Name]
			}
			if !found {
				continue
			}

			entry := timers[serviceName]
			entry.mu.Lock()
			if entry.timer != nil {
				entry.timer.Stop()
			}
			// Capture for closure.
			svcName := serviceName
			entry.timer = time.AfterFunc(w.debounce, func() {
				w.restart(ctx, svcName)
			})
			entry.mu.Unlock()

		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			w.logger.WithError(err).Warn("watcher error")
		}
	}
}

// restart stops then starts the named service so the new code is picked up.
func (w *Watcher) restart(ctx context.Context, serviceName string) {
	log := w.logger.WithField("service", serviceName)
	log.Info("change detected — restarting service")

	if err := w.scheduler.StopService(ctx, serviceName); err != nil {
		log.WithError(err).Error("failed to stop service for hot reload")
		return
	}

	// Put service name in context (required by health checks).
	restartCtx := context.WithValue(ctx, "service", serviceName)
	if err := w.scheduler.StartService(restartCtx, serviceName); err != nil {
		log.WithError(err).Error("failed to restart service after hot reload")
		return
	}

	log.Info("service restarted successfully")
}
