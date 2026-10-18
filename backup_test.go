package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"yuexi/internal/db"
)

// sqliteMagic 是每个合法 SQLite 数据库文件开头 16 字节的固定文件头。
var sqliteMagic = []byte("SQLite format 3\x00")

// TestRunBackupSnapshot 走应用正常初始化路径建一个真实库并插入一行，然后经
// runBackup 导出快照：产物必须以 SQLite 文件头开头，且重新打开能读回该行。
// 导出时原库连接保持打开（模拟运行中的进程），验证 WAL 里已提交的数据同样
// 出现在快照中。
func TestRunBackupSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yuexi.db")

	prev := db.DB
	t.Cleanup(func() {
		if db.DB != prev {
			db.DB.Close()
			db.DB = prev
		}
	})
	db.Init(path)

	if _, err := db.DB.Exec(
		`INSERT INTO persons (user_id, name, cycle_length, period_length) VALUES (1, ?, 28, 5)`,
		"备份测试"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("YUEXI_DB_PATH", path)
	var buf bytes.Buffer
	if err := runBackup(&buf); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	// (a) 前 16 字节必须恰好是 SQLite 文件头。
	if got := buf.Bytes(); len(got) < 16 || !bytes.Equal(got[:16], sqliteMagic) {
		t.Fatalf("快照文件头 = %q..., 期望 %q", got[:min(16, len(got))], sqliteMagic)
	}

	// (b) 重新打开快照，行数据能原样读回。
	snapPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(snapPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	snap, err := sql.Open("sqlite", snapPath)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	var name string
	if err := snap.QueryRow(`SELECT name FROM persons WHERE name = ?`, "备份测试").Scan(&name); err != nil {
		t.Fatalf("快照中读不回插入的行: %v", err)
	}
	if name != "备份测试" {
		t.Errorf("name = %q, 期望 %q", name, "备份测试")
	}
}

// TestRunBackupMissingFile 验证 backup 不会顺手创建缺失的数据库文件：
// 导出进程遇到路径配错时只报错，绝不产生一个空库。
func TestRunBackupMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "yuexi.db")
	t.Setenv("YUEXI_DB_PATH", path)

	if err := runBackup(&bytes.Buffer{}); err == nil {
		t.Fatal("runBackup 对不存在的数据库应报错")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("backup 不应创建缺失的数据库文件, stat err = %v", err)
	}
}

// TestRunBackupNoMigrate 验证导出路径不迁移、不改动 schema：对一个只有
// 一张无关表的库跑 backup 后，原库的 sqlite_master 里不应出现任何应用表。
func TestRunBackupNoMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// 建一个不含应用 schema 的最小库。
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE legacy_only (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("YUEXI_DB_PATH", path)
	if err := runBackup(&bytes.Buffer{}); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	after, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	var cnt int
	if err := after.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('users','persons','records','sessions','daily_logs','notification_config')`,
	).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Errorf("backup 后原库出现 %d 张应用表，导出路径不应执行迁移", cnt)
	}
}
