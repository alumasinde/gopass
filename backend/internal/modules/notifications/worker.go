package notifications

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}
type Worker struct {
	db    *sql.DB
	cfg   SMTPConfig
	log   *slog.Logger
	poll  time.Duration
	batch int
}

func NewWorker(db *sql.DB, cfg SMTPConfig, log *slog.Logger) *Worker {
	return &Worker{db: db, cfg: cfg, log: log, poll: 2 * time.Second, batch: 10}
}
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.ProcessBatch(ctx); err != nil && w.log != nil {
				w.log.ErrorContext(ctx, "notification worker batch failed", "error", err)
			}
		}
	}
}
func (w *Worker) ProcessBatch(ctx context.Context) error {
	for i := 0; i < w.batch; i++ {
		n, err := w.claim(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		sendErr := w.send(ctx, n)
		if sendErr != nil {
			if err := w.fail(ctx, n, sendErr); err != nil {
				return err
			}
			continue
		}
		if err := w.complete(ctx, n.ID); err != nil {
			return err
		}
	}
	return nil
}

type delivery struct {
	ID, OrgID                          int64
	Event, Recipient, Subject, Payload string
	Attempts, MaxAttempts              int
}

func (w *Worker) claim(ctx context.Context) (delivery, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return delivery{}, err
	}
	defer tx.Rollback()
	// Recover abandoned claims after worker crashes, then claim exactly one due row.
	if _, err = tx.ExecContext(ctx, `UPDATE notifications SET status=CASE WHEN attempts>=max_attempts THEN 'DEAD' ELSE 'RETRY' END,claimed_at=NULL,updated_at=UTC_TIMESTAMP() WHERE status='PROCESSING' AND claimed_at < DATE_SUB(UTC_TIMESTAMP(),INTERVAL 10 MINUTE)`); err != nil {
		return delivery{}, err
	}
	var n delivery
	err = tx.QueryRowContext(ctx, `SELECT id,organization_id,event_type,recipient,subject,CAST(payload AS CHAR),attempts,max_attempts FROM notifications WHERE status IN ('PENDING','RETRY') AND next_attempt_at<=UTC_TIMESTAMP() AND attempts<max_attempts ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&n.ID, &n.OrgID, &n.Event, &n.Recipient, &n.Subject, &n.Payload, &n.Attempts, &n.MaxAttempts)
	if err != nil {
		return delivery{}, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE notifications SET status='PROCESSING',attempts=attempts+1,claimed_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE id=? AND status IN ('PENDING','RETRY')`, n.ID)
	if err != nil {
		return delivery{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return delivery{}, err
	}
	if affected != 1 {
		return delivery{}, sql.ErrNoRows
	}
	if err = tx.Commit(); err != nil {
		return delivery{}, err
	}
	n.Attempts++
	return n, nil
}
func (w *Worker) send(ctx context.Context, n delivery) error {
	if w.cfg.Host == "" || w.cfg.Port == "" || w.cfg.From == "" {
		return errors.New("SMTP is not configured (SMTP_HOST, SMTP_PORT, SMTP_FROM required)")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.ContainsAny(n.Recipient, "\r\n") || strings.ContainsAny(n.Subject, "\r\n") || strings.ContainsAny(w.cfg.From, "\r\n") {
		return errors.New("invalid email header")
	}
	from, err := mail.ParseAddress(w.cfg.From)
	if err != nil {
		return errors.New("SMTP_FROM must be a valid email address")
	}
	recipient, err := mail.ParseAddress(n.Recipient)
	if err != nil {
		return errors.New("invalid recipient email address")
	}
	addr := net.JoinHostPort(w.cfg.Host, w.cfg.Port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	client, err := smtp.NewClient(conn, w.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: w.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	} else if w.cfg.Username != "" || w.cfg.Password != "" {
		return errors.New("SMTP server does not advertise STARTTLS; refusing to send credentials over plaintext")
	}
	if w.cfg.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", w.cfg.Username, w.cfg.Password, w.cfg.Host)); err != nil {
			return err
		}
	}
	if err = client.Mail(from.Address); err != nil {
		return err
	}
	if err = client.Rcpt(recipient.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nGoPass notification: %s\nDetails: %s\r\n", w.cfg.From, recipient.Address, n.Subject, n.Event, n.Payload)
	if _, err = io.WriteString(writer, body); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
func (w *Worker) complete(ctx context.Context, id int64) error {
	_, err := w.db.ExecContext(ctx, `UPDATE notifications SET status='DELIVERED',delivered_at=UTC_TIMESTAMP(),claimed_at=NULL,last_error=NULL,updated_at=UTC_TIMESTAMP() WHERE id=? AND status='PROCESSING'`, id)
	return err
}
func (w *Worker) fail(ctx context.Context, n delivery, sendErr error) error {
	msg := sendErr.Error()
	if len(msg) > 950 {
		msg = msg[:950]
	}
	if n.Attempts >= n.MaxAttempts {
		_, err := w.db.ExecContext(ctx, `UPDATE notifications SET status='DEAD',claimed_at=NULL,last_error=?,updated_at=UTC_TIMESTAMP() WHERE id=? AND status='PROCESSING'`, msg, n.ID)
		return err
	}
	delay := backoffSeconds(n.Attempts)
	_, err := w.db.ExecContext(ctx, `UPDATE notifications SET status='RETRY',claimed_at=NULL,last_error=?,next_attempt_at=DATE_ADD(UTC_TIMESTAMP(),INTERVAL ? SECOND),updated_at=UTC_TIMESTAMP() WHERE id=? AND status='PROCESSING'`, msg, delay, n.ID)
	if w.log != nil {
		w.log.WarnContext(ctx, "notification delivery failed; retry scheduled", "notification_id", n.ID, "attempt", n.Attempts, "error", strings.TrimSpace(msg), "retry_seconds", delay)
	}
	return err
}
