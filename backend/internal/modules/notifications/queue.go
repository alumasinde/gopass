package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"strings"
)

type Message struct {
	OrganizationID int64
	EventType      string
	Recipient      string
	Subject        string
	Payload        any
	IdempotencyKey string
}

// EnqueueTx adds a durable email delivery to the same transaction as the
// business event. If the business transaction rolls back, the notification does too.
func EnqueueTx(ctx context.Context, tx *sql.Tx, m Message) error {
	if tx == nil {
		return errors.New("notification transaction is nil")
	}
	if m.OrganizationID < 1 || strings.TrimSpace(m.EventType) == "" || strings.TrimSpace(m.Recipient) == "" || strings.TrimSpace(m.Subject) == "" {
		return errors.New("invalid notification")
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(m.Recipient))
	if err != nil {
		return fmt.Errorf("invalid notification recipient: %w", err)
	}
	body, err := json.Marshal(m.Payload)
	if err != nil {
		return fmt.Errorf("encode notification payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO notifications(organization_id,event_type,channel,recipient,subject,payload,status,attempts,max_attempts,next_attempt_at,idempotency_key,created_at,updated_at) VALUES(?,?,'EMAIL',?,?,?,'PENDING',0,8,UTC_TIMESTAMP(),?,UTC_TIMESTAMP(),UTC_TIMESTAMP()) ON DUPLICATE KEY UPDATE id=id`, m.OrganizationID, m.EventType, addr.Address, m.Subject, body, nullableKey(m.IdempotencyKey))
	return err
}
func nullableKey(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
func backoffSeconds(attempt int) int {
	if attempt < 1 {
		attempt = 1
	}
	n := 30 * math.Pow(2, float64(attempt-1))
	if n > 21600 {
		n = 21600
	}
	return int(n)
}
