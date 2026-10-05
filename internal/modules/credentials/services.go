package credentials

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"strings"
	"time"
)



type Service struct {
	db    *sql.DB
	authz *rbac.Service
}

func NewService(db *sql.DB, a *rbac.Service) *Service { return &Service{db, a} }
func token() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func (s *Service) List(c context.Context, o int64) ([]Credential, error) {
	rows, e := s.db.QueryContext(c, `SELECT id,organization_id,gatepass_id,token,expires_at,is_revoked FROM credentials WHERE organization_id=? ORDER BY id DESC LIMIT 200`, o)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []Credential
	for rows.Next() {
		var v Credential
		var t time.Time
		if e := rows.Scan(&v.ID, &v.OrganizationID, &v.GatepassID, &v.Token, &t, &v.IsRevoked); e != nil {
			return nil, e
		}
		v.ExpiresAt = t.UTC().Format(time.RFC3339)
		x = append(x, v)
	}
	return x, rows.Err()
}
func (s *Service) Issue(c context.Context, o, id int64) (*Credential, error) {
	var status string
	var exp time.Time
	var gate int64
	e := s.db.QueryRowContext(c, `SELECT status,valid_until,gate_id FROM gatepasses WHERE organization_id=? AND id=?`, o, id).Scan(&status, &exp, &gate)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	if status != "APPROVED" {
		return nil, ErrUnavailable
	}
	var n int
	e = s.db.QueryRowContext(c, `SELECT COUNT(*) FROM visitors v JOIN gatepasses gp ON gp.visitor_id=v.id WHERE gp.organization_id=? AND gp.id=? AND v.is_blacklisted=0`, o, id).Scan(&n)
	if e != nil || n == 0 {
		return nil, ErrUnavailable
	}
	tok := token()
	res, e := s.db.ExecContext(c, `INSERT INTO credentials(organization_id,gatepass_id,token,expires_at,is_revoked,created_at,updated_at) VALUES(?,?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, id, tok, exp)
	if e != nil {
		return nil, e
	}
	cid, _ := res.LastInsertId()
	_, e = s.db.ExecContext(c, `UPDATE gatepasses SET status='ISSUED',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='APPROVED'`, o, id)
	if e != nil {
		return nil, e
	}
	return &Credential{ID: cid, OrganizationID: o, GatepassID: id, Token: tok, ExpiresAt: exp.UTC().Format(time.RFC3339)}, nil
}
func (s *Service) Verify(c context.Context, o string) (*Credential, error) {
	var v Credential
	var exp time.Time
	var oid, gp, gate int64
	var revoked bool
	var vf, vu time.Time
	err := s.db.QueryRowContext(c, `SELECT c.id,c.organization_id,c.gatepass_id,c.token,c.expires_at,c.is_revoked,gp.gate_id,gp.valid_from,gp.valid_until FROM credentials c JOIN gatepasses gp ON gp.id=c.gatepass_id WHERE c.organization_id=? AND c.token=?`, func() int64 {
		if cl, ok := auth.From(c); ok {
			return cl.OrgID
		}
		return 0
	}(), strings.TrimSpace(o)).Scan(&v.ID, &oid, &gp, &v.Token, &exp, &revoked, &gate, &vf, &vu)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.OrganizationID = oid
	v.GatepassID = gp
	v.ExpiresAt = exp.UTC().Format(time.RFC3339)
	v.IsRevoked = revoked
	if revoked || time.Now().UTC().After(exp) || time.Now().UTC().Before(vf) || time.Now().UTC().After(vu) {
		return nil, ErrUnavailable
	}
	var status string
	if e := s.db.QueryRowContext(c, `SELECT status FROM gatepasses WHERE organization_id=? AND id=?`, oid, gp).Scan(&status); e != nil {
		return nil, e
	}
	if status != "ISSUED" && status != "CHECKED_IN" {
		return nil, ErrUnavailable
	}
	_ = gate
	return &v, nil
}
func (s *Service) Revoke(c context.Context, o, id int64) error {
	res, e := s.db.ExecContext(c, `UPDATE credentials SET is_revoked=1,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND is_revoked=0`, o, id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
