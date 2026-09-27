package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://urlshortner:urlshortner123@localhost:5432/urlshortner?sslmode=disable"
	if v := os.Getenv("DB_DSN"); v != "" {
		dsn = v
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal("cannot connect:", err)
	}

	var userID int64
	if err := db.QueryRowContext(ctx, "SELECT id FROM users WHERE display_user_id = 'USR_9ZL2zWWN4im'").Scan(&userID); err != nil {
		log.Fatal("user USR_9ZL2zWWN4im not found:", err)
	}
	fmt.Printf("User id=%d\n", userID)

	var destID int64
	err = db.QueryRowContext(ctx,
		`INSERT INTO destinations (original_url, url_hash)
		 VALUES ('https://example.com/seed-test', encode(sha256('https://example.com/seed-test'::bytea), 'hex'))
		 ON CONFLICT DO NOTHING
		 RETURNING id`).Scan(&destID)
	if err != nil {
		_ = db.QueryRowContext(ctx, "SELECT id FROM destinations WHERE original_url = 'https://example.com/seed-test'").Scan(&destID)
	}
	fmt.Printf("Destination id=%d\n", destID)

	var urlID int64
	err = db.QueryRowContext(ctx,
		`INSERT INTO urls (user_id, short_code, destination_id, title, click_count, url_status, last_health_check, last_accessed_at)
		 VALUES ($1, 'test20clicks', $2, 'Seed: 20 clicks test', 20, 1, NOW(), NOW())
		 ON CONFLICT (short_code) DO UPDATE SET click_count = 20, user_id = $1
		 RETURNING id`, userID, destID).Scan(&urlID)
	if err != nil {
		_ = db.QueryRowContext(ctx, "SELECT id FROM urls WHERE short_code = 'test20clicks'").Scan(&urlID)
	}
	fmt.Printf("URL id=%d\n", urlID)

	_, _ = db.ExecContext(ctx, "DELETE FROM click_logs WHERE url_id = $1", urlID)

	type agent struct {
		ip, ua, ref, browser, device string
	}

	agents10 := []agent{
		{"203.0.113.10", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"203.0.113.11", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"198.51.100.5", "Mozilla/5.0 Safari/17", "https://bing.com", "Safari", "Desktop"},
		{"203.0.113.10", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"192.0.2.22", "Mozilla/5.0 Firefox/121", "https://reddit.com", "Firefox", "Desktop"},
		{"203.0.113.12", "Mozilla/5.0 Chrome/120", "", "Chrome", "Mobile"},
		{"198.51.100.5", "Mozilla/5.0 Safari/17", "https://bing.com", "Safari", "Desktop"},
		{"203.0.113.10", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"192.0.2.55", "Mozilla/5.0 Edge/120", "https://twitter.com", "Edge", "Desktop"},
		{"203.0.113.11", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
	}

	agents5m := []agent{
		{"198.51.100.8", "Mozilla/5.0 Safari/17", "https://linkedin.com", "Safari", "Mobile"},
		{"203.0.113.13", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"192.0.2.99", "Mozilla/5.0 Firefox/121", "https://reddit.com", "Firefox", "Desktop"},
		{"203.0.113.10", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"198.51.100.5", "Mozilla/5.0 Safari/17", "https://bing.com", "Safari", "Desktop"},
	}

	agentsFast := []agent{
		{"203.0.113.14", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"203.0.113.14", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"192.0.2.33", "Mozilla/5.0 Firefox/121", "https://reddit.com", "Firefox", "Mobile"},
		{"203.0.113.10", "Mozilla/5.0 Chrome/120", "https://google.com", "Chrome", "Desktop"},
		{"198.51.100.5", "Mozilla/5.0 Safari/17", "https://bing.com", "Safari", "Desktop"},
	}

	now := time.Now().UTC()
	total := 0

	for i, a := range agents10 {
		ts := now.Add(-30*time.Minute + time.Duration(i)*time.Minute)
		ref := sql.NullString{String: a.ref, Valid: a.ref != ""}
		_, err := db.ExecContext(ctx,
			`INSERT INTO click_logs (url_id, clicked_at, ip_address, user_agent, referrer, browser, device_type)
			 VALUES ($1, $2, $3::inet, $4, $5, $6, $7)`, urlID, ts, a.ip, a.ua, ref, a.browser, a.device)
		if err != nil {
			log.Printf("click %d error: %v", i+1, err)
			continue
		}
		total++
	}

	gaps := []time.Duration{18, 15, 12, 9, 6}
	for i, a := range agents5m {
		ts := now.Add(-gaps[i] * time.Minute)
		ref := sql.NullString{String: a.ref, Valid: a.ref != ""}
		_, err := db.ExecContext(ctx,
			`INSERT INTO click_logs (url_id, clicked_at, ip_address, user_agent, referrer, browser, device_type)
			 VALUES ($1, $2, $3::inet, $4, $5, $6, $7)`, urlID, ts, a.ip, a.ua, ref, a.browser, a.device)
		if err != nil {
			log.Printf("click %d error: %v", 10+i+1, err)
			continue
		}
		total++
	}

	for i, a := range agentsFast {
		ts := now.Add(-5*time.Minute + time.Duration(i)*4*time.Second)
		ref := sql.NullString{String: a.ref, Valid: a.ref != ""}
		_, err := db.ExecContext(ctx,
			`INSERT INTO click_logs (url_id, clicked_at, ip_address, user_agent, referrer, browser, device_type)
			 VALUES ($1, $2, $3::inet, $4, $5, $6, $7)`, urlID, ts, a.ip, a.ua, ref, a.browser, a.device)
		if err != nil {
			log.Printf("click %d error: %v", 15+i+1, err)
			continue
		}
		total++
	}

	fmt.Printf("Inserted %d click_logs for URL id=%d (click_count=20)\n", total, urlID)
}
