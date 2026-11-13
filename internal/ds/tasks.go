package ds

import (
	"context"
	"errors"
	"fmt"
	"github.com/fomk/389-ds-ui/internal/actor"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/events"
)

// Task types: the container under cn=tasks,cn=config.
const (
	TaskIndex   = "index"
	TaskBackup  = "backup"
	TaskRestore = "restore"
	TaskExport  = "export"
	TaskImport  = "import"
	// TaskCleanAllRUV removes a replica ID from every RUV of the topology.
	TaskCleanAllRUV = "cleanallruv"
)

// TaskStatus is the lifecycle state of a task.
type TaskStatus string

const (
	TaskRunning   TaskStatus = "running"
	TaskSucceeded TaskStatus = "succeeded"
	TaskFailed    TaskStatus = "failed"
	TaskUnknown   TaskStatus = "unknown" // the task entry disappeared before it reported an exit code
)

// Task is a server task started by ds-ui.
type Task struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	DN       string     `json:"dn"`
	Target   string     `json:"target,omitempty" doc:"What the task works on (backend, file, attributes)"`
	Status   TaskStatus `json:"status" enum:"running,succeeded,failed,unknown"`
	Message  string     `json:"message,omitempty" doc:"Last nsTaskStatus"`
	ExitCode *int       `json:"exitCode,omitempty"`
	Current  int        `json:"current,omitempty"`
	Total    int        `json:"total,omitempty"`
	Log      []string   `json:"log"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	By       string     `json:"by,omitempty" doc:"DN of the signed-in user who started it"`
}

// Doer runs a function with an LDAP connection.
type Doer interface {
	Do(ctx context.Context, fn func(*ldap.Conn) error) error
}

// Tasks starts 389-ds tasks (entries under cn=tasks,cn=config) and follows
// them until they finish, publishing every change on the event bus.
type Tasks struct {
	ldap     Doer
	bus      *events.Bus
	log      *slog.Logger
	interval time.Duration

	mu    sync.Mutex
	tasks []*Task // newest last; at most maxTasks
}

const maxTasks = 100

// NewTasks creates the task manager.
func NewTasks(l Doer, bus *events.Bus, log *slog.Logger) *Tasks {
	if log == nil {
		log = slog.Default()
	}
	if bus == nil {
		bus = &events.Bus{}
	}
	return &Tasks{ldap: l, bus: bus, log: log, interval: time.Second}
}

// Start creates a task entry and follows it in the background.
func (m *Tasks) Start(ctx context.Context, typ, target string, attrs map[string][]string) (*Task, error) {
	now := time.Now().UTC()
	name := fmt.Sprintf("%s_%s", strings.ReplaceAll(typ, " ", "_"), now.Format("20060102_150405.000000"))
	dn := "cn=" + ldap.EscapeDN(name) + ",cn=" + ldap.EscapeDN(typ) + ",cn=tasks,cn=config"
	req := ldap.NewAddRequest(dn, nil)
	req.Attribute("objectClass", []string{"top", "extensibleObject"})
	req.Attribute("cn", []string{name})
	// Keep the finished entry for an hour so its log stays readable.
	req.Attribute("ttl", []string{"3600"})
	for k, v := range attrs {
		req.Attribute(k, v)
	}
	if err := m.ldap.Do(ctx, func(c *ldap.Conn) error { return c.Add(req) }); err != nil {
		return nil, err
	}
	t := &Task{ID: name, Type: typ, DN: dn, Target: target, Status: TaskRunning, Log: []string{}, Started: now, By: actor.Name(ctx)}
	m.mu.Lock()
	m.tasks = append(m.tasks, t)
	if len(m.tasks) > maxTasks {
		m.tasks = m.tasks[len(m.tasks)-maxTasks:]
	}
	m.mu.Unlock()
	m.publish(t)
	go m.follow(t)
	return m.snapshot(t), nil
}

// follow polls the task entry until it reports an exit code.
func (m *Tasks) follow(t *Task) {
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	tick := time.NewTicker(m.interval)
	defer tick.Stop()
	missing := 0
	for range tick.C {
		var e *ldap.Entry
		err := m.ldap.Do(ctx, func(c *ldap.Conn) (err error) {
			e, err = readEntry(c, t.DN, "nsTaskStatus", "nsTaskLog", "nsTaskExitCode", "nsTaskCurrentItem", "nsTaskTotalItems")
			return err
		})
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			// Restore restarts the backend; very fast tasks may expire first.
			if missing++; missing < 5 {
				continue
			}
			m.finish(t, TaskUnknown, nil)
			return
		}
		if err != nil {
			if ctx.Err() != nil {
				m.finish(t, TaskUnknown, nil)
				return
			}
			continue // server restarting, connection lost: keep trying
		}
		changed := m.update(t, e)
		if code := e.GetAttributeValue("nsTaskExitCode"); code != "" {
			n, _ := strconv.Atoi(code)
			status := TaskSucceeded
			if n != 0 {
				status = TaskFailed
			}
			m.finish(t, status, &n)
			return
		}
		if changed {
			m.publish(t)
		}
	}
}

func (m *Tasks) update(t *Task, e *ldap.Entry) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := fmt.Sprint(t.Message, t.Current, t.Total, len(t.Log))
	t.Message = e.GetAttributeValue("nsTaskStatus")
	t.Current, _ = strconv.Atoi(e.GetAttributeValue("nsTaskCurrentItem"))
	t.Total, _ = strconv.Atoi(e.GetAttributeValue("nsTaskTotalItems"))
	if l := e.GetAttributeValue("nsTaskLog"); l != "" {
		t.Log = strings.Split(strings.TrimRight(l, "\n"), "\n")
	}
	return before != fmt.Sprint(t.Message, t.Current, t.Total, len(t.Log))
}

func (m *Tasks) finish(t *Task, status TaskStatus, code *int) {
	now := time.Now().UTC()
	m.mu.Lock()
	t.Status, t.ExitCode, t.Finished = status, code, &now
	m.mu.Unlock()
	m.log.Info("task finished", "task", t.ID, "status", status)
	m.publish(t)
}

func (m *Tasks) publish(t *Task) {
	m.bus.Publish(events.Event{Type: "task", Data: m.snapshot(t)})
}

func (m *Tasks) snapshot(t *Task) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *t
	c.Log = slices.Clone(t.Log)
	return &c
}

// Busy reports whether a task of the given type is running on target
// (case-insensitive), as far as this ds-ui started it.
func (m *Tasks) Busy(typ, target string) bool {
	if m == nil {
		return false
	}
	for _, t := range m.List() {
		if t.Status == TaskRunning && t.Type == typ && strings.EqualFold(t.Target, target) {
			return true
		}
	}
	return false
}

// List returns the tasks started by this ds-ui instance, newest first.
func (m *Tasks) List() []*Task {
	m.mu.Lock()
	tasks := slices.Clone(m.tasks)
	m.mu.Unlock()
	out := make([]*Task, 0, len(tasks))
	for i := len(tasks) - 1; i >= 0; i-- {
		out = append(out, m.snapshot(tasks[i]))
	}
	return out
}

// ErrTaskNotFound is returned for an unknown task id.
var ErrTaskNotFound = errors.New("task not found")

// Get returns one task.
func (m *Tasks) Get(id string) (*Task, error) {
	m.mu.Lock()
	var found *Task
	for _, t := range m.tasks {
		if t.ID == id {
			found = t
		}
	}
	m.mu.Unlock()
	if found == nil {
		return nil, ErrTaskNotFound
	}
	return m.snapshot(found), nil
}

// Wait blocks until the task finishes or ctx is done (used by tests).
func (m *Tasks) Wait(ctx context.Context, id string) (*Task, error) {
	for {
		t, err := m.Get(id)
		if err != nil || t.Status != TaskRunning {
			return t, err
		}
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Operations recorded in the task history although they are not server
// tasks (they finish immediately): deletions, uploads, restarts.
const (
	OpDeleteBackup  = "delete-backup"
	OpUploadLDIF    = "upload-ldif"
	OpDeleteLDIF    = "delete-ldif"
	OpCreateBackend = "create-backend"
	OpDeleteBackend = "delete-backend"
	OpRestart       = "restart"
	OpEnablePlugin  = "enable-plugin"
	OpDisablePlugin = "disable-plugin"
	OpRootPassword  = "root-password"
	OpLockAccount   = "lock-account"
	OpUnlockAccount = "unlock-account"
	OpInitReplica   = "init-replica"
	OpCreatePlugin  = "create-plugin"
	OpDeletePlugin  = "delete-plugin"
	OpEncryptAttr   = "encrypt-attr"
	OpDecryptAttr   = "decrypt-attr"
	// OpCertFromFiles: a server certificate imported from the watched files.
	OpCertFromFiles = "cert-from-files"
)

// Record adds a finished operation to the history: ok when err is nil,
// failed with the error as message otherwise.
func (m *Tasks) Record(ctx context.Context, typ, target string, err error) {
	if m == nil {
		return
	}
	now := time.Now().UTC()
	t := &Task{
		ID: fmt.Sprintf("%s_%s", typ, now.Format("20060102_150405.000000")), Type: typ, Target: target,
		Status: TaskSucceeded, Log: []string{}, Started: now, Finished: &now, By: actor.Name(ctx),
	}
	if err != nil {
		t.Status, t.Message = TaskFailed, err.Error()
	}
	m.mu.Lock()
	m.tasks = append(m.tasks, t)
	if len(m.tasks) > maxTasks {
		m.tasks = m.tasks[len(m.tasks)-maxTasks:]
	}
	m.mu.Unlock()
	m.publish(t)
}
