package service

import (
	"fmt"
	"log"
	"time"
	"yuexi/internal/db"

	"github.com/containrrr/shoutrrr"
)

var notifyTicker *time.Ticker
var notifyDone chan struct{}

func StartNotificationChecker() {
	notifyDone = make(chan struct{})
	go func() {
		// Check every 30 minutes
		ticker := time.NewTicker(30 * time.Minute)
		notifyTicker = ticker
		checkNotifications()
		for {
			select {
			case <-ticker.C:
				checkNotifications()
			case <-notifyDone:
				ticker.Stop()
				return
			}
		}
	}()
	log.Println("通知检查服务已启动")
}

func StopNotificationChecker() {
	if notifyDone != nil {
		close(notifyDone)
	}
}

func checkNotifications() {
	// Get all users who have notifications enabled
	userIDs := getAllNotificationUserIDs()

	today := time.Now()
	todayStr := today.Format("2006-01-02")

	for _, userID := range userIDs {
		cfg := db.GetNotificationConfig(userID)
		if !cfg.Enabled || cfg.ShoutrrrURL == "" {
			continue
		}

		// Don't send more than once per day
		if cfg.LastNotified == todayStr {
			continue
		}

		persons, err := db.GetPersonsByUser(userID)
		if err != nil {
			continue
		}

		for _, p := range persons {
			records, err := db.GetRecordsByPerson(p.ID)
			if err != nil || len(records) == 0 {
				continue
			}

			nextPeriod := GetNextPeriodDate(p, records)
			if nextPeriod == nil {
				continue
			}

			daysUntil := int(nextPeriod.Sub(today).Hours() / 24)

			if daysUntil >= 0 && daysUntil <= cfg.DaysBefore {
				msg := fmt.Sprintf("月汐提醒：%s 的月经预计在 %d 天后到来（%s）",
					p.Name, daysUntil, nextPeriod.Format("2006-01-02"))

				if err := sendNotification(cfg.ShoutrrrURL, msg); err != nil {
					log.Printf("通知发送失败: %v", err)
					continue
				}
				log.Printf("通知已发送: %s", msg)
			}

			// Check for cycle anomalies
			anomalies := DetectCycleAnomaly(p, records)
			for _, anomaly := range anomalies {
				msg := fmt.Sprintf("月汐提醒：%s 的周期异常 - %s", p.Name, anomaly.Description)
				if err := sendNotification(cfg.ShoutrrrURL, msg); err != nil {
					log.Printf("异常通知发送失败: %v", err)
				}
			}
		}

		db.UpdateNotificationLastNotified(userID, todayStr)
	}
}

func getAllNotificationUserIDs() []int64 {
	rows, err := db.DB.Query("SELECT user_id FROM notification_config WHERE enabled = 1")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func sendNotification(url, message string) error {
	sender, err := shoutrrr.NewSender(log.Default(), url)
	if err != nil {
		return fmt.Errorf("创建发送器失败: %w", err)
	}

	errs := sender.Send(message, nil)
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func TestNotification(url string) error {
	return sendNotification(url, "月汐测试通知：连接成功！")
}
