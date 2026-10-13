package audit

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
)

var auditDriverSequence atomic.Uint64

type captureDriver struct{ capture *execCapture }
type execCapture struct {
	query string
	args  []driver.NamedValue
}
type captureConn struct{ capture *execCapture }

func (d captureDriver) Open(string) (driver.Conn, error)  { return captureConn{capture: d.capture}, nil }
func (c captureConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c captureConn) Close() error                        { return nil }
func (c captureConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c captureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.capture.query = query
	c.capture.args = append([]driver.NamedValue(nil), args...)
	return driver.RowsAffected(1), nil
}

func TestRecordUsesAuthenticatedTenantAndActor(t *testing.T) {
	capture := &execCapture{}
	driverName := fmt.Sprintf("gopass-audit-capture-test-%d", auditDriverSequence.Add(1))
	sql.Register(driverName, captureDriver{capture: capture})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := tenancy.With(context.Background(), 22)
	ctx = auth.With(ctx, &auth.Claims{UserID: 11, OrgID: 22})
	New(db, slog.New(slog.NewTextHandler(io.Discard, nil))).Record(ctx, Entry{
		OrganizationID: 999, ActorUserID: 888, Action: "role.updated", ResourceType: "role", ResourceID: 5,
		OldValues: map[string]any{"active": false}, NewValues: map[string]any{"active": true},
	})
	if !strings.Contains(capture.query, "INSERT INTO audit_logs") {
		t.Fatalf("unexpected query: %s", capture.query)
	}
	if len(capture.args) != 7 {
		t.Fatalf("got %d args, want 7", len(capture.args))
	}
	if got := capture.args[0].Value; got != int64(22) {
		t.Fatalf("organization_id=%v, want context tenant 22", got)
	}
	if got := capture.args[1].Value; got != int64(11) {
		t.Fatalf("actor_user_id=%v, want authenticated actor 11", got)
	}
	if got := capture.args[2].Value; got != "role.updated" {
		t.Fatalf("action=%v", got)
	}
	if got := capture.args[3].Value; got != "role" {
		t.Fatalf("resource_type=%v", got)
	}
	if got := capture.args[4].Value; got != int64(5) {
		t.Fatalf("resource_id=%v", got)
	}
	for _, index := range []int{5, 6} {
		b, ok := capture.args[index].Value.([]byte)
		if !ok {
			t.Fatalf("JSON argument %d has type %T, want []byte", index, capture.args[index].Value)
		}
		var value map[string]any
		if err := json.Unmarshal(b, &value); err != nil {
			t.Fatalf("invalid JSON audit value: %v", err)
		}
	}
}
