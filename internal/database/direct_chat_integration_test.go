package database

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGetOrCreateDirectChatConcurrent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	schema := "direct_chat_test_" + rand.Text()
	schemaName := pgx.Identifier{schema}.Sanitize()
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName
	// One connection holds the lock, two run the repository, and one observes them.
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.MinIdleConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open PostgreSQL pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})

	for _, path := range []string{
		"../../migrations/00001_init_schema.sql",
		"../../migrations/00004_create_direct_chats.sql",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		up, _, ok := strings.Cut(string(data), "-- +goose Down")
		if !ok {
			t.Fatalf("missing Down marker in %s", path)
		}
		if _, err := pool.Exec(ctx, up, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}

	var firstUser, secondUser int
	err = pool.QueryRow(ctx, "INSERT INTO users (username) VALUES ('first') RETURNING id").Scan(&firstUser)
	if err != nil {
		t.Fatalf("create first user: %v", err)
	}
	err = pool.QueryRow(ctx, "INSERT INTO users (username) VALUES ('second') RETURNING id").Scan(&secondUser)
	if err != nil {
		t.Fatalf("create second user: %v", err)
	}

	gate, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}
	defer gate.Rollback(ctx)
	// Hold both requests at INSERT INTO chats, after each has missed the initial SELECT.
	if _, err := gate.Exec(ctx, "LOCK TABLE chats IN SHARE MODE"); err != nil {
		t.Fatalf("lock chats: %v", err)
	}

	type result struct {
		chat Chat
		err  error
	}
	results := make(chan result, 2)
	repo := NewRepository(pool)
	for _, pair := range [][2]int{{firstUser, secondUser}, {secondUser, firstUser}} {
		go func() {
			chat, err := repo.GetOrCreateDirectChat(ctx, pair[0], pair[1])
			results <- result{chat, err}
		}()
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM pg_locks
			WHERE locktype = 'relation'
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND relation = 'chats'::regclass
			  AND mode = 'RowExclusiveLock'
			  AND NOT granted
		`).Scan(&waiting)
		if err != nil {
			t.Fatalf("observe concurrent inserts: %v", err)
		}
		if waiting == 2 {
			break
		}
		select {
		case res := <-results:
			t.Fatalf("request finished before both inserts reached the lock: chat %d, error %v", res.chat.ID, res.err)
		case <-ctx.Done():
			t.Fatalf("both inserts did not reach the lock: %v", ctx.Err())
		case <-ticker.C:
		}
	}
	if err := gate.Commit(ctx); err != nil {
		t.Fatalf("release chats lock: %v", err)
	}

	var chatID int
	for i := 0; i < 2; i++ {
		select {
		case res := <-results:
			if res.err != nil {
				t.Fatalf("open direct chat: %v", res.err)
			}
			if res.chat.ID <= 0 || (i == 1 && res.chat.ID != chatID) {
				t.Fatalf("expected both requests to return the same positive chat ID, got %d and %d", chatID, res.chat.ID)
			}
			chatID = res.chat.ID
		case <-ctx.Done():
			t.Fatalf("wait for chat requests: %v", ctx.Err())
		}
	}

	var chats, pairs, members int
	err = pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM chats),
		       (SELECT count(*) FROM direct_chats WHERE chat_id = $1 AND user_low_id = $2 AND user_high_id = $3),
		       (SELECT count(*) FROM chat_members WHERE chat_id = $1 AND user_id IN ($2, $3))
	`, chatID, firstUser, secondUser).Scan(&chats, &pairs, &members)
	if err != nil {
		t.Fatalf("read committed chat data: %v", err)
	}
	if chats != 1 || pairs != 1 || members != 2 {
		t.Errorf("found %d chats, %d matching pairs and %d participants, want 1, 1 and 2", chats, pairs, members)
	}

	var thirdUser int
	if err := pool.QueryRow(ctx, "INSERT INTO users (username) VALUES ('third') RETURNING id").Scan(&thirdUser); err != nil {
		t.Fatalf("create third user: %v", err)
	}
	if err := repo.AddChatMember(ctx, chatID, thirdUser); !errors.Is(err, ErrChatMembersNotEditable) {
		t.Fatalf("add third user to direct chat: got %v, want ErrChatMembersNotEditable", err)
	}
	isMember, err := repo.IsChatMember(ctx, chatID, thirdUser)
	if err != nil || isMember {
		t.Fatalf("third user membership: got %t, %v, want false, nil", isMember, err)
	}
	groupChat, err := repo.CreateChat(ctx)
	if err != nil {
		t.Fatalf("create regular chat: %v", err)
	}
	if err := repo.AddChatMember(ctx, groupChat.ID, thirdUser); err != nil {
		t.Fatalf("add user to regular chat: %v", err)
	}
	isMember, err = repo.IsChatMember(ctx, groupChat.ID, thirdUser)
	if err != nil || !isMember {
		t.Fatalf("regular chat membership: got %t, %v, want true, nil", isMember, err)
	}
}
