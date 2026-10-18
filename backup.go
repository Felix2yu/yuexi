package main

import (
	"io"

	"yuexi/internal/db"
)

// runBackup 实现 `yuexi backup`：把一份 SQLite 一致性快照（VACUUM INTO）写到
// w（由调用方给 os.Stdout），供外部备份工具（docker-db-auto-backup）经
// docker exec 在容器内调用并捕获 stdout 作为产物。
//
// 日志一律走 stderr（标准 log 默认即 stderr），stdout 只承载快照字节。
func runBackup(w io.Writer) error {
	sqlDB, err := db.OpenForExport(dbPathFromEnv())
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	return db.WriteSQLiteSnapshot(sqlDB, w)
}
