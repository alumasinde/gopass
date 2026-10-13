package checkins

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
)

type Service struct {
	db    *sql.DB
	authz *rbac.Service
}

func NewService(db *sql.DB, a *rbac.Service) *Service { return &Service{db, a} }

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func (s *Service) List(c context.Context, o int64) ([]CheckIn, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,gatepass_id,gate_id,checked_in_by,checked_in_at FROM check_ins WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []CheckIn
	for rows.Next() {
		var v CheckIn
		var t time.Time
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.GatepassID, &v.GateID, &v.CheckedInBy, &t); e != nil {
			return nil, e
		}
		v.CheckedInAt = t.UTC().Format(time.RFC3339)
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Create(c context.Context, o int64, token string, gate int64) (*CheckIn, error) {
	cl, ok := auth.From(c)
	if !ok {
		return nil, ErrInvalid
	}
	if e := s.authz.RequireWithScope(c, "checkins.perform", rbac.ScopeGate, gate); e != nil {
		return nil, e
	}
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var gpID, gpGate int64
	var status string
	var validFrom, validUntil time.Time
	var exp time.Time
	tokHash := hashToken(token)
	err := tx.QueryRowContext(c, `SELECT gp.id,gp.gate_id,gp.status,gp.valid_from,gp.valid_until,cr.expires_at FROM credentials cr JOIN gatepasses gp ON gp.id=cr.gatepass_id WHERE cr.organization_id=? AND cr.token_hash=? AND cr.is_revoked=0 FOR UPDATE`, o, tokHash).Scan(&gpID, &gpGate, &status, &validFrom, &validUntil, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	// Check validity window
	if validFrom.After(now) || validUntil.Before(now) {
		return nil, ErrInvalid
	}
	// Check expiration
	if now.After(exp) {
		return nil, ErrInvalid
	}
	if gpGate != gate || status != "ISSUED" {
		return nil, ErrInvalid
	}
	// Recheck visitor blacklist
	var isBlacklisted bool
	err = tx.QueryRowContext(c, `SELECT v.is_blacklisted FROM gatepasses gp JOIN visitors v ON v.id=gp.visitor_id WHERE gp.id=?`, gpID).Scan(&isBlacklisted)
	if err != nil {
		return nil, err
	}
	if isBlacklisted {
		return nil, ErrInvalid
	}
	var n int
	if err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM check_ins WHERE organization_id=? AND gatepass_id=?`, o, gpID).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrAlready
	}
	res, err := tx.ExecContext(c, `INSERT INTO check_ins(organization_id,gatepass_id,gate_id,checked_in_by,checked_in_at) VALUES(?,?,?,?,UTC_TIMESTAMP())`, o, gpID, gate, cl.UserID)
	if err != nil {
		return nil, err
	}
	res2, err := tx.ExecContext(c, `UPDATE gatepasses SET status='CHECKED_IN',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='ISSUED'`, o, gpID)
	if err != nil {
		return nil, err
	}
	if n, _ := res2.RowsAffected(); n != 1 {
		return nil, ErrInvalid
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	nowStr := now.Format(time.RFC3339)
	return &CheckIn{ID: id, OrganizationID: o, GatepassID: gpID, GateID: gate, CheckedInBy: &cl.UserID, CheckedInAt: nowStr}, nil
}
