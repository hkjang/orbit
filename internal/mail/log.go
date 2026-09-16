package mail

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresLog 는 발송 기록을 mail_deliveries 표에 둔다.
type PostgresLog struct{ db *pgxpool.Pool }

func NewPostgresLog(db *pgxpool.Pool) *PostgresLog { return &PostgresLog{db: db} }

func (l *PostgresLog) Record(ctx context.Context, d Delivery) error {
	_, err := l.db.Exec(ctx, `INSERT INTO mail_deliveries(id,event,recipient,subject,actor_id,resource_type,resource_id,status,attempts,created_at,updated_at) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,'queued',0,$8,$8)`,
		d.ID, d.Event, d.Recipient, d.Subject, d.ActorID, d.ResourceType, d.ResourceID, d.CreatedAt)
	return err
}

func (l *PostgresLog) Complete(ctx context.Context, d Delivery) error {
	_, err := l.db.Exec(ctx, `UPDATE mail_deliveries SET status=$2,attempts=GREATEST(attempts,$3),error_message=$4,updated_at=$5 WHERE id=$1`,
		d.ID, d.Status, d.Attempts, d.ErrorMessage, d.UpdatedAt)
	return err
}

func (l *PostgresLog) RecentlyNotified(ctx context.Context, event, recipient string, within time.Duration) (bool, error) {
	var recent bool
	err := l.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mail_deliveries WHERE event=$1 AND lower(recipient)=lower($2) AND status IN ('queued','sent') AND created_at > now() - make_interval(secs => $3))`,
		event, recipient, within.Seconds()).Scan(&recent)
	return recent, err
}

func (l *PostgresLog) List(ctx context.Context, status string, limit int) (Page, error) {
	page := Page{Items: []Delivery{}, Summary: Summary{Status: map[string]int{}}}
	rows, err := l.db.Query(ctx, `SELECT id::text,event,recipient,subject,coalesce(actor_id::text,''),resource_type,resource_id,status,attempts,error_message,created_at,updated_at FROM mail_deliveries WHERE ($1='' OR status=$1) ORDER BY created_at DESC, id LIMIT $2`, status, limit)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Delivery
		if err := rows.Scan(&item.ID, &item.Event, &item.Recipient, &item.Subject, &item.ActorID, &item.ResourceType, &item.ResourceID,
			&item.Status, &item.Attempts, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	counts, err := l.db.Query(ctx, `SELECT status, count(*) FROM mail_deliveries GROUP BY 1`)
	if err != nil {
		return Page{}, err
	}
	defer counts.Close()
	for counts.Next() {
		var key string
		var count int
		if err := counts.Scan(&key, &count); err != nil {
			return Page{}, err
		}
		page.Summary.Status[key] = count
		page.Summary.Total += count
	}
	return page, counts.Err()
}
