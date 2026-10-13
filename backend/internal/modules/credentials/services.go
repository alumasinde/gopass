package credentials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/alumasinde/gopass/internal/app/database"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
)

type Service struct {
	db    database.Handle
	authz *rbac.Service
	audit *audit.Service
}

func NewService(db any, a *rbac.Service, audits ...*audit.Service) *Service {
	var au *audit.Service
	if len(audits) > 0 {
		au = audits[0]
	}
	return &Service{db: database.Adapt(db), authz: a, audit: au}
}
func token() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func (s *Service) List(c context.Context, o int64) ([]Credential, error) {
	all, gateIDs, err := s.authz.AccessibleGates(c, "credentials.view")
	if err != nil {
		return nil, err
	}
	if !all && len(gateIDs) == 0 {
		return []Credential{}, nil
	}
	query := `SELECT c.id,c.organization_id,c.gatepass_id,c.expires_at,c.is_revoked,gp.gate_id FROM credentials c JOIN gatepasses gp ON gp.id=c.gatepass_id AND gp.organization_id=c.organization_id WHERE c.organization_id=?`
	args := []any{o}
	if !all {
		ph := strings.TrimSuffix(strings.Repeat("?,", len(gateIDs)), ",")
		query += ` AND gp.gate_id IN (` + ph + `)`
		for _, id := range gateIDs {
			args = append(args, id)
		}
	}
	query += ` ORDER BY c.id DESC LIMIT 200`
	rows, err := s.db.QueryContext(c, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Credential
	for rows.Next() {
		var v Credential
		var t time.Time
		var gateID int64
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.GatepassID, &t, &v.IsRevoked, &gateID); err != nil {
			return nil, err
		}
		v.ExpiresAt = t.UTC().Format(time.RFC3339)
		v.Token = ""
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) Issue(c context.Context, o, id int64) (*Credential, error) {
	tx, e := s.db.BeginTx(c, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var status string
	var exp time.Time
	var gate int64
	var visitor int64
	err := tx.QueryRowContext(c, `SELECT status,valid_until,gate_id,visitor_id FROM gatepasses WHERE organization_id=? AND id=? FOR UPDATE`, o, id).Scan(&status, &exp, &gate, &visitor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "APPROVED" {
		return nil, ErrUnavailable
	}
	if time.Now().UTC().After(exp) {
		return nil, ErrUnavailable
	}
	if e := s.authz.RequireWithScope(c, "credentials.issue", rbac.ScopeGate, gate); e != nil {
		return nil, e
	}
	var blacklisted bool
	if err = tx.QueryRowContext(c, `SELECT is_blacklisted FROM visitors WHERE organization_id=? AND id=? LOCK IN SHARE MODE`, o, visitor).Scan(&blacklisted); err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, ErrUnavailable
	}
	var existing int
	if err = tx.QueryRowContext(c, `SELECT COUNT(*) FROM credentials WHERE organization_id=? AND gatepass_id=? AND is_revoked=0`, o, id).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrUnavailable
	}
	tok := token()
	tokHash := hashToken(tok)
	res, err := tx.ExecContext(c, `INSERT INTO credentials(organization_id,gatepass_id,token_hash,expires_at,is_revoked,created_at,updated_at) VALUES(?,?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, o, id, tokHash, exp)
	if err != nil {
		return nil, err
	}
	cid, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	res, err = tx.ExecContext(c, `UPDATE gatepasses SET status='ISSUED',updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND status='APPROVED'`, o, id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrUnavailable
	}
	if s.audit != nil {
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "credential.issued", ResourceType: "credential", ResourceID: cid, NewValues: map[string]any{"gatepass_id": id, "expires_at": exp.UTC(), "is_revoked": false}}); err != nil {
			return nil, err
		}
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "gatepass.issued", ResourceType: "gatepass", ResourceID: id, OldValues: map[string]any{"status": "APPROVED"}, NewValues: map[string]any{"status": "ISSUED"}}); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	// Only return the token once at issue time
	return &Credential{ID: cid, OrganizationID: o, GatepassID: id, Token: tok, ExpiresAt: exp.UTC().Format(time.RFC3339)}, nil
}
func (s *Service) Verify(c context.Context, tokenValue string) (*Credential, error) {
	var v Credential
	var exp time.Time
	var oid, gp, gate int64
	var revoked bool
	var vf, vu time.Time
	cl, ok := auth.From(c)
	if !ok {
		return nil, ErrUnavailable
	}
	tokHash := hashToken(strings.TrimSpace(tokenValue))
	err := s.db.QueryRowContext(c, `SELECT c.id,c.organization_id,c.gatepass_id,c.expires_at,c.is_revoked,gp.gate_id,gp.valid_from,gp.valid_until FROM credentials c JOIN gatepasses gp ON gp.id=c.gatepass_id AND gp.organization_id=c.organization_id WHERE c.organization_id=? AND c.token_hash=?`, cl.OrgID, tokHash).Scan(&v.ID, &oid, &gp, &exp, &revoked, &gate, &vf, &vu)
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
	v.Token = "" // Never return token in verify
	if revoked || time.Now().UTC().After(exp) || time.Now().UTC().Before(vf) || time.Now().UTC().After(vu) {
		return nil, ErrUnavailable
	}
	if e := s.authz.RequireWithScope(c, "credentials.verify", rbac.ScopeGate, gate); e != nil {
		return nil, e
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
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var gateID, gatepassID int64
	var revoked bool
	err = tx.QueryRowContext(c, `SELECT gp.gate_id,cr.gatepass_id,cr.is_revoked FROM credentials cr JOIN gatepasses gp ON gp.id=cr.gatepass_id AND gp.organization_id=cr.organization_id WHERE cr.organization_id=? AND cr.id=? FOR UPDATE`, o, id).Scan(&gateID, &gatepassID, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if revoked {
		return ErrNotFound
	}
	if err = s.authz.RequireWithScope(c, "credentials.revoke", rbac.ScopeGate, gateID); err != nil {
		return err
	}
	res, err := tx.ExecContext(c, `UPDATE credentials SET is_revoked=1,updated_at=UTC_TIMESTAMP() WHERE organization_id=? AND id=? AND is_revoked=0`, o, id)
	if err != nil {
		return err
	}
	if n, e := res.RowsAffected(); e != nil || n != 1 {
		if e != nil {
			return e
		}
		return ErrNotFound
	}
	if s.audit != nil {
		if err = s.audit.RecordTx(c, tx, audit.Entry{OrganizationID: o, Action: "credential.revoked", ResourceType: "credential", ResourceID: id, OldValues: map[string]any{"is_revoked": false}, NewValues: map[string]any{"is_revoked": true, "gatepass_id": gatepassID}}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
