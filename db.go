package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var storage Storage

type Storage interface {
	CreateOrUpdateProfile(p Profile) error
	GetProfile(userID int64) (*Profile, error)
	CreateOrder(o Order) (int64, error)
	GetOrderByCreator(userID int64) (*Order, error)
	GetOrderByID(id int64) (*Order, error)
	DeleteOrderByID(id int64) error
	UpdateOrder(o Order) error
	IncrementComplaint(orderID int64, reporterID int64) (int, error)
	ListOrdersByCategory(cat string) ([]Order, error)
	Close() error
}

////////////////////////////////////////////////////////////////////////////////
// JSON file fallback (not recommended for production)
////////////////////////////////////////////////////////////////////////////////

type JSONStorage struct {
	FilePath string
	mu       sync.Mutex
	Data     struct {
		Profiles map[int64]Profile ` + "`json:\"profiles\"`" + `
		Orders   map[int64]Order   ` + "`json:\"orders\"`" + `
		NextID   int64             ` + "`json:\"next_id\"`" + `
	}
}

func InitJSONStorage(path string) error {
	js := &JSONStorage{FilePath: path}
	js.Data.Profiles = map[int64]Profile{}
	js.Data.Orders = map[int64]Order{}
	js.Data.NextID = 1
	// load if exists
	if _, err := os.Stat(path); err == nil {
		b, _ := os.ReadFile(path)
		_ = json.Unmarshal(b, &js.Data)
	}
	storage = js
	return nil
}

func (j *JSONStorage) persist() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	b, err := json.MarshalIndent(j.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(j.FilePath, b, 0644)
}

func (j *JSONStorage) CreateOrUpdateProfile(p Profile) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Data.Profiles[p.UserID] = p
	return j.persist()
}
func (j *JSONStorage) GetProfile(userID int64) (*Profile, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if p, ok := j.Data.Profiles[userID]; ok {
		return &p, nil
	}
	return nil, errors.New("not found")
}

func (j *JSONStorage) CreateOrder(o Order) (int64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	// ensure creator has no active order
	for _, od := range j.Data.Orders {
		if od.CreatorID == o.CreatorID {
			return 0, errors.New("creator already has an order")
		}
	}
	id := j.Data.NextID
	o.ID = id
	j.Data.Orders[id] = o
	j.Data.NextID++
	_ = j.persist()
	return id, nil
}
func (j *JSONStorage) GetOrderByCreator(userID int64) (*Order, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, od := range j.Data.Orders {
		if od.CreatorID == userID {
			temp := od
			return &temp, nil
		}
	}
	return nil, errors.New("not found")
}
func (j *JSONStorage) GetOrderByID(id int64) (*Order, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if od, ok := j.Data.Orders[id]; ok {
		temp := od
		return &temp, nil
	}
	return nil, errors.New("not found")
}
func (j *JSONStorage) DeleteOrderByID(id int64) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.Data.Orders, id)
	return j.persist()
}
func (j *JSONStorage) UpdateOrder(o Order) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Data.Orders[o.ID] = o
	return j.persist()
}
func (j *JSONStorage) IncrementComplaint(orderID int64, reporterID int64) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	od, ok := j.Data.Orders[orderID]
	if !ok {
		return 0, errors.New("not found")
	}
	od.Complaints++
	j.Data.Orders[orderID] = od
	_ = j.persist()
	return od.Complaints, nil
}
func (j *JSONStorage) ListOrdersByCategory(cat string) ([]Order, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []Order
	for _, od := range j.Data.Orders {
		if od.Category == cat {
			out = append(out, od)
		}
	}
	return out, nil
}
func (j *JSONStorage) Close() error { return nil }

////////////////////////////////////////////////////////////////////////////////
// Postgres implementation (simple)
// NOTE: minimal; errors bubbled up
////////////////////////////////////////////////////////////////////////////////

var pgpool *pgxpool.Pool

func InitPostgres(databaseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	pgpool = pool
	// create tables
	_, err = pgpool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS profiles (
 user_id BIGINT PRIMARY KEY,
 username TEXT,
 description TEXT,
 photo_file_id TEXT,
 updated_at TIMESTAMP DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS orders (
 id BIGSERIAL PRIMARY KEY,
 creator_id BIGINT,
 category TEXT,
 text TEXT,
 photo_file_id TEXT,
 group_message_id BIGINT,
 complaints INT DEFAULT 0,
 created_at TIMESTAMP DEFAULT NOW()
);
`)
	if err != nil {
		return err
	}
	storage = &PostgresStorage{}
	return nil
}

type PostgresStorage struct{}

func (p *PostgresStorage) CreateOrUpdateProfile(pr Profile) error {
	ctx := context.Background()
	_, err := pgpool.Exec(ctx, `INSERT INTO profiles (user_id, username, description, photo_file_id, updated_at)
 VALUES ($1,$2,$3,$4,$5)
 ON CONFLICT (user_id) DO UPDATE SET username=EXCLUDED.username, description=EXCLUDED.description, photo_file_id=EXCLUDED.photo_file_id, updated_at=EXCLUDED.updated_at
`, pr.UserID, pr.Username, pr.Description, pr.PhotoFileID, time.Now())
	return err
}
func (p *PostgresStorage) GetProfile(userID int64) (*Profile, error) {
	ctx := context.Background()
	var pr Profile
	err := pgpool.QueryRow(ctx, `SELECT user_id, username, description, photo_file_id FROM profiles WHERE user_id=$1`, userID).Scan(&pr.UserID, &pr.Username, &pr.Description, &pr.PhotoFileID)
	if err != nil {
		return nil, err
	}
	return &pr, nil
}
func (p *PostgresStorage) CreateOrder(o Order) (int64, error) {
	ctx := context.Background()
	// check existing order by creator
	var exists bool
	err := pgpool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE creator_id=$1)`, o.CreatorID).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, errors.New("creator already has an order")
	}
	var id int64
	err = pgpool.QueryRow(ctx, `INSERT INTO orders (creator_id, category, text, photo_file_id) VALUES ($1,$2,$3,$4) RETURNING id`, o.CreatorID, o.Category, o.Text, o.PhotoFileID).Scan(&id)
	return id, err
}
func (p *PostgresStorage) GetOrderByCreator(userID int64) (*Order, error) {
	ctx := context.Background()
	var o Order
	err := pgpool.QueryRow(ctx, `SELECT id, creator_id, category, text, photo_file_id, complaints FROM orders WHERE creator_id=$1`, userID).Scan(&o.ID, &o.CreatorID, &o.Category, &o.Text, &o.PhotoFileID, &o.Complaints)
	if err != nil {
		return nil, err
	}
	return &o, nil
}
func (p *PostgresStorage) GetOrderByID(id int64) (*Order, error) {
	ctx := context.Background()
	var o Order
	err := pgpool.QueryRow(ctx, `SELECT id, creator_id, category, text, photo_file_id, complaints FROM orders WHERE id=$1`, id).Scan(&o.ID, &o.CreatorID, &o.Category, &o.Text, &o.PhotoFileID, &o.Complaints)
	if err != nil {
		return nil, err
	}
	return &o, nil
}
func (p *PostgresStorage) DeleteOrderByID(id int64) error {
	ctx := context.Background()
	_, err := pgpool.Exec(ctx, `DELETE FROM orders WHERE id=$1`, id)
	return err
}
func (p *PostgresStorage) UpdateOrder(o Order) error {
	ctx := context.Background()
	_, err := pgpool.Exec(ctx, `UPDATE orders SET category=$1, text=$2, photo_file_id=$3 WHERE id=$4`, o.Category, o.Text, o.PhotoFileID, o.ID)
	return err
}
func (p *PostgresStorage) IncrementComplaint(orderID int64, reporterID int64) (int, error) {
	ctx := context.Background()
	_, err := pgpool.Exec(ctx, `INSERT INTO orders (id) SELECT id WHERE id=$1`)
	if err != nil {
		// ignore
	}
	_, err = pgpool.Exec(ctx, `UPDATE orders SET complaints = complaints + 1 WHERE id=$1`, orderID)
	if err != nil {
		return 0, err
	}
	var c int
	err = pgpool.QueryRow(ctx, `SELECT complaints FROM orders WHERE id=$1`, orderID).Scan(&c)
	return c, err
}
func (p *PostgresStorage) ListOrdersByCategory(cat string) ([]Order, error) {
	ctx := context.Background()
	rows, err := pgpool.Query(ctx, `SELECT id, creator_id, category, text, photo_file_id, complaints FROM orders WHERE category=$1`, cat)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Order
	for rows.Next() {
		var o Order
		rows.Scan(&o.ID, &o.CreatorID, &o.Category, &o.Text, &o.PhotoFileID, &o.Complaints)
		out = append(out, o)
	}
	return out, nil
}
func (p *PostgresStorage) Close() error {
	if pgpool != nil {
		pgpool.Close()
	}
	return nil
}
