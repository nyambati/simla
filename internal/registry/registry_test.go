package registry

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// newTestRegistry builds an in-memory ServiceRegistry backed by a temp file.
func newTestRegistry(t *testing.T) *ServiceRegistry {
	t.Helper()
	dir := t.TempDir()
	return &ServiceRegistry{
		Services:          make(map[string]*Service),
		BasePort:          9000,
		LastAllocatedPort: 8999,
		FilePath:          filepath.Join(dir, "registry.yaml"),
		logger:            logrus.NewEntry(logrus.New()),
		mutex:             &sync.RWMutex{},
	}
}

var ctx = context.Background()

// ── AddService ────────────────────────────────────────────────────────────────

func TestAddService_NewService_AllocatesPort(t *testing.T) {
	r := newTestRegistry(t)
	svc, err := r.AddService(ctx, "payments")
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, "payments", svc.Name)
	assert.Equal(t, StatusPending, svc.Status)
	assert.Equal(t, 9000, svc.Port) // first allocation: 8999+1
}

func TestAddService_SecondService_AllocatesNextPort(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")
	svc, err := r.AddService(ctx, "orders")
	require.NoError(t, err)
	assert.Equal(t, 9001, svc.Port)
}

func TestAddService_ExistingService_ReturnsCachedNoSave(t *testing.T) {
	r := newTestRegistry(t)
	first, err := r.AddService(ctx, "payments")
	require.NoError(t, err)

	// Remove the registry file so any Save attempt would fail if called.
	require.NoError(t, os.Remove(r.FilePath))

	second, err := r.AddService(ctx, "payments")
	require.NoError(t, err, "should not attempt disk write for existing service")
	assert.Equal(t, first.Port, second.Port)
	assert.Same(t, first, second, "should be the same pointer")
}

func TestAddService_PersistsToFile(t *testing.T) {
	r := newTestRegistry(t)
	_, err := r.AddService(ctx, "payments")
	require.NoError(t, err)

	data, err := os.ReadFile(r.FilePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "payments")
}

// ── GetService ────────────────────────────────────────────────────────────────

func TestGetService_Found(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")

	svc, ok := r.GetService(ctx, "payments")
	require.True(t, ok)
	assert.Equal(t, "payments", svc.Name)
}

func TestGetService_NotFound(t *testing.T) {
	r := newTestRegistry(t)
	svc, ok := r.GetService(ctx, "missing")
	assert.False(t, ok)
	assert.Nil(t, svc)
}

// ── ListServices ──────────────────────────────────────────────────────────────

func TestListServices_Empty(t *testing.T) {
	r := newTestRegistry(t)
	list := r.ListServices(ctx)
	assert.Empty(t, list)
}

func TestListServices_ReturnsAll(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")
	_, _ = r.AddService(ctx, "orders")
	_, _ = r.AddService(ctx, "auth")

	list := r.ListServices(ctx)
	assert.Len(t, list, 3)

	names := make(map[string]bool)
	for _, s := range list {
		names[s.Name] = true
	}
	assert.True(t, names["payments"])
	assert.True(t, names["orders"])
	assert.True(t, names["auth"])
}

// ── UpdateStatus ──────────────────────────────────────────────────────────────

func TestUpdateStatus_KnownService(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")

	r.UpdateStatus(ctx, "payments", StatusRunning)
	svc, _ := r.GetService(ctx, "payments")
	assert.Equal(t, StatusRunning, svc.Status)
}

func TestUpdateStatus_UnknownService_NoePanic(t *testing.T) {
	r := newTestRegistry(t)
	// Must not panic.
	assert.NotPanics(t, func() {
		r.UpdateStatus(ctx, "ghost", StatusRunning)
	})
}

func TestUpdateStatus_AllStatuses(t *testing.T) {
	for _, status := range []Status{StatusPending, StatusRunning, StatusStopped, StatusFailed} {
		status := status
		t.Run(string(status), func(t *testing.T) {
			r := newTestRegistry(t)
			_, _ = r.AddService(ctx, "svc")
			r.UpdateStatus(ctx, "svc", status)
			svc, _ := r.GetService(ctx, "svc")
			assert.Equal(t, status, svc.Status)
		})
	}
}

// ── UpdateHealth ──────────────────────────────────────────────────────────────

func TestUpdateHealth_KnownService(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")

	r.UpdateHealth(ctx, "payments", true)
	svc, _ := r.GetService(ctx, "payments")
	assert.True(t, svc.Healthy)

	r.UpdateHealth(ctx, "payments", false)
	svc, _ = r.GetService(ctx, "payments")
	assert.False(t, svc.Healthy)
}

func TestUpdateHealth_UnknownService_NoPanic(t *testing.T) {
	r := newTestRegistry(t)
	assert.NotPanics(t, func() {
		r.UpdateHealth(ctx, "ghost", true)
	})
}

// ── UpdateContainerID ─────────────────────────────────────────────────────────

func TestUpdateContainerID_KnownService(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")

	err := r.UpdateContainerID(ctx, "payments", "abc123")
	require.NoError(t, err)

	svc, _ := r.GetService(ctx, "payments")
	assert.Equal(t, "abc123", svc.ID)
}

func TestUpdateContainerID_UnknownService_ReturnsError(t *testing.T) {
	r := newTestRegistry(t)
	err := r.UpdateContainerID(ctx, "ghost", "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestUpdateContainerID_PersistsToFile(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")
	require.NoError(t, r.UpdateContainerID(ctx, "payments", "containerXYZ"))

	data, err := os.ReadFile(r.FilePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "containerXYZ")
}

// ── Save / Load ───────────────────────────────────────────────────────────────

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "payments")
	_, _ = r.AddService(ctx, "orders")
	_ = r.UpdateContainerID(ctx, "payments", "ctr-abc")

	// Load into a fresh registry from the same file.
	r2 := newTestRegistry(t)
	r2.FilePath = r.FilePath
	require.NoError(t, r2.Load(ctx))

	assert.Equal(t, r.LastAllocatedPort, r2.LastAllocatedPort)
	assert.Len(t, r2.Services, 2)

	svc, ok := r2.GetService(ctx, "payments")
	require.True(t, ok)
	assert.Equal(t, "ctr-abc", svc.ID)
	assert.Equal(t, "payments", svc.Name)
}

func TestLoad_FileNotExist_InitialisesEmpty(t *testing.T) {
	r := newTestRegistry(t)
	// File doesn't exist yet; Load should create the directory and return nil.
	require.NoError(t, r.Load(ctx))
	assert.Empty(t, r.Services)
}

func TestLoad_BackfillsNameField(t *testing.T) {
	// Write a registry file that lacks the Name field (simulates older format).
	dir := t.TempDir()
	filePath := filepath.Join(dir, "registry.yaml")

	raw := map[string]interface{}{
		"lastAllocatedPort": 9000,
		"services": map[string]interface{}{
			"payments": map[string]interface{}{"port": 9000, "id": "ctr-1"},
		},
	}
	data, err := yaml.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filePath, data, 0644))

	r := &ServiceRegistry{
		FilePath: filePath,
		logger:   logrus.NewEntry(logrus.New()),
		mutex:    &sync.RWMutex{},
	}
	require.NoError(t, r.Load(ctx))

	svc, ok := r.GetService(ctx, "payments")
	require.True(t, ok)
	assert.Equal(t, "payments", svc.Name, "Name should be backfilled from map key")
}

func TestLoad_CorruptFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "registry.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(":::invalid yaml:::"), 0644))

	r := &ServiceRegistry{
		FilePath: filePath,
		logger:   logrus.NewEntry(logrus.New()),
		mutex:    &sync.RWMutex{},
	}
	err := r.Load(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load registry")
}

func TestSave_UnwritablePath_ReturnsError(t *testing.T) {
	r := newTestRegistry(t)
	r.FilePath = "/this/path/does/not/exist/registry.yaml"
	err := r.Save(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save registry")
}

// ── Port allocation monotonicity ──────────────────────────────────────────────

func TestPortAllocation_Monotonic(t *testing.T) {
	r := newTestRegistry(t)
	ports := make([]int, 5)
	names := []string{"a", "b", "c", "d", "e"}
	for i, name := range names {
		svc, err := r.AddService(ctx, name)
		require.NoError(t, err)
		ports[i] = svc.Port
	}
	for i := 1; i < len(ports); i++ {
		assert.Greater(t, ports[i], ports[i-1], "ports should be strictly increasing")
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestAddService_ConcurrentCalls_NoPanic(t *testing.T) {
	r := newTestRegistry(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "svc"
			if n%2 == 0 {
				name = "other"
			}
			_, _ = r.AddService(ctx, name)
		}(i)
	}
	wg.Wait()
}

func TestUpdateStatus_ConcurrentCalls_NoPanic(t *testing.T) {
	r := newTestRegistry(t)
	_, _ = r.AddService(ctx, "svc")

	var wg sync.WaitGroup
	statuses := []Status{StatusRunning, StatusPending, StatusStopped, StatusFailed}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.UpdateStatus(ctx, "svc", statuses[i%len(statuses)])
		}(i)
	}
	wg.Wait()
}
