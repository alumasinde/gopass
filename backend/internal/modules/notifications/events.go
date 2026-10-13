package notifications

import (
	"context"
	"database/sql"
	"net/mail"
	"strings"
)

// EnqueueVisitorTx safely skips email delivery when the visitor has no email.
// In-app/SMS channels can be added later without changing workflow transactions.
func EnqueueVisitorTx(ctx context.Context, tx *sql.Tx, orgID, gatepassID int64, event, subject string, payload map[string]any) error {
	var email sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT v.email FROM gatepasses gp JOIN visitors v ON v.id=gp.visitor_id AND v.organization_id=gp.organization_id WHERE gp.organization_id=? AND gp.id=?`, orgID, gatepassID).Scan(&email)
	if err != nil {
		return err
	}
	if !email.Valid || strings.TrimSpace(email.String) == "" {
		return nil
	}
	if _, parseErr := mail.ParseAddress(strings.TrimSpace(email.String)); parseErr != nil {
		return nil
	}
	key := event + ":" + fmtInt(gatepassID)
	return EnqueueTx(ctx, tx, Message{OrganizationID: orgID, EventType: event, Recipient: email.String, Subject: subject, Payload: payload, IdempotencyKey: key})
}
func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// EnqueueApproversTx notifies active users whose current RBAC assignment grants
// approval for the gate. Delivery rows are idempotent per approver and gatepass.
func EnqueueApproversTx(ctx context.Context, tx *sql.Tx, orgID, gatepassID, gateID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT u.id,u.email FROM users u JOIN user_roles ur ON ur.user_id=u.id AND ur.organization_id=u.organization_id JOIN roles r ON r.id=ur.role_id AND r.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=r.id JOIN permissions p ON p.id=rp.permission_id WHERE u.organization_id=? AND u.is_active=1 AND ur.is_active=1 AND p.code='approvals.approve' AND u.email<>'' AND (ur.scope_type='ORGANIZATION' OR (ur.scope_type='GATE' AND ur.gate_id=?) OR (ur.scope_type='SITE' AND ur.site_id=(SELECT site_id FROM gates WHERE organization_id=? AND id=?)) )`, orgID, gateID, orgID, gateID)
	if err != nil {
		return err
	}
	type approver struct {
		id    int64
		email string
	}
	var recipients []approver
	for rows.Next() {
		var a approver
		if err := rows.Scan(&a.id, &a.email); err != nil {
			rows.Close()
			return err
		}
		recipients = append(recipients, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, a := range recipients {
		if _, err := mail.ParseAddress(strings.TrimSpace(a.email)); err != nil {
			continue
		}
		key := "approval.requested:" + fmtInt(gatepassID) + ":approver:" + fmtInt(a.id)
		if err := EnqueueTx(ctx, tx, Message{OrganizationID: orgID, EventType: "approval.requested", Recipient: a.email, Subject: "Gatepass approval required", Payload: map[string]any{"gatepass_id": gatepassID, "gate_id": gateID, "status": "PENDING_APPROVAL"}, IdempotencyKey: key}); err != nil {
			return err
		}
	}
	return nil
}
