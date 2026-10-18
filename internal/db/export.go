package db

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// OpenForExport 以导出模式打开数据库，与 Init 的区别：不创建目录、不跑迁移、
// 不做任何种子写入，也不设置 journal_mode——导出连接不该改写库头部，已有的
// WAL 数据照常读取。数据文件不存在时直接报错：导出进程永远不该顺手新建空库。
func OpenForExport(dbPath string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("数据文件不可用: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 导出是顺序读，一条连接足够，避免多余连接各自打开 -shm 句柄。
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	return sqlDB, nil
}

// WriteSQLiteSnapshot 把数据库以 SQLite 自身的一致性快照写到 w。
//
// 用 VACUUM INTO 而不是复制 .db / -wal / -shm 文件：热拷贝正在被写入的文件
// 不是原子操作，周期性拷出的副本可能损坏或漏掉 WAL 里已提交的数据；
// VACUUM INTO 由 SQLite 在事务内生成一份自洽的快照，WAL 模式下也不阻塞写入。
//
// 快照先落在临时目录再流式输出：VACUUM INTO 要求目标不存在，且这样无论
// 调用方读到一半断开，都不会在数据目录里留下半成品。
func WriteSQLiteSnapshot(sqlDB *sql.DB, w io.Writer) error {
	dir, err := os.MkdirTemp("", "yuexi-snapshot-")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(dir)

	target := filepath.Join(dir, "snapshot.db")
	if _, err := sqlDB.Exec("VACUUM INTO ?", target); err != nil {
		return fmt.Errorf("导出数据库快照失败: %w", err)
	}

	f, err := os.Open(target)
	if err != nil {
		return fmt.Errorf("读取快照失败: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("写出快照失败: %w", err)
	}
	return nil
}
