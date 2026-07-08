package handler

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"
	"yuexi/internal/db"

	"golang.org/x/crypto/bcrypt"
)

var sessionKey = make([]byte, 32)

func init() {
	rand.Read(sessionKey)
	// Clean up expired sessions periodically
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			db.DeleteExpiredSessions()
		}
	}()
}

// Login rate limiting
type loginAttempt struct {
	count    int
	lastTry  time.Time
	blockedUntil time.Time
}

var (
	loginAttempts = make(map[string]*loginAttempt)
	loginMu       sync.Mutex
)

func checkLoginRateLimit(ip string) (allowed bool, retryAfter int) {
	loginMu.Lock()
	defer loginMu.Unlock()

	now := time.Now()
	attempt, exists := loginAttempts[ip]

	if !exists {
		loginAttempts[ip] = &loginAttempt{count: 1, lastTry: now}
		return true, 0
	}

	// If blocked, check if block period has expired
	if !attempt.blockedUntil.IsZero() && now.Before(attempt.blockedUntil) {
		retry := int(attempt.blockedUntil.Sub(now).Seconds()) + 1
		return false, retry
	}

	// Reset if last attempt was more than 15 minutes ago
	if now.Sub(attempt.lastTry) > 15*time.Minute {
		attempt.count = 1
		attempt.lastTry = now
		attempt.blockedUntil = time.Time{}
		return true, 0
	}

	attempt.count++
	attempt.lastTry = now

	// Block after 5 failed attempts
	if attempt.count >= 5 {
		attempt.blockedUntil = now.Add(15 * time.Minute)
		return false, 900
	}

	return true, 0
}

func resetLoginAttempts(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginAttempts, ip)
}

func createSession(userID int64, username string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	expiresAt := time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	db.CreateSession(token, userID, username, expiresAt)
	return token
}

func getSession(r *http.Request) *db.SessionData {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	sess, err := db.GetSession(cookie.Value)
	if err != nil || sess == nil {
		return nil
	}
	return sess
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := getSession(r)
		if sess == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func injectUser(r *http.Request, data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = make(map[string]interface{})
	}
	sess := getSession(r)
	if sess != nil {
		data["CurrentUser"] = sess.Username
	}
	return data
}

func GetUserID(r *http.Request) int64 {
	sess := getSession(r)
	if sess == nil {
		return 0
	}
	return sess.UserID
}

func LoginPage(w http.ResponseWriter, r *http.Request) {
	if getSession(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	tmpl, _ := parseTemplates("layout.html", "login.html")
	tmpl.ExecuteTemplate(w, "layout", map[string]interface{}{
		"Error": r.URL.Query().Get("error"),
	})
}

func RegisterPage(w http.ResponseWriter, r *http.Request) {
	if getSession(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	tmpl, _ := parseTemplates("layout.html", "register.html")
	tmpl.ExecuteTemplate(w, "layout", map[string]interface{}{
		"Error":         r.URL.Query().Get("error"),
		"UserCount":     db.UserCount(),
	})
}

func LoginPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
	}

	allowed, _ := checkLoginRateLimit(ip)
	if !allowed {
		http.Redirect(w, r, "/login?error=登录尝试过多，请稍后再试", http.StatusSeeOther)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if username == "" || password == "" {
		http.Redirect(w, r, "/login?error=请输入用户名和密码", http.StatusSeeOther)
		return
	}

	user, err := db.GetUserByUsername(username)
	if err != nil {
		http.Redirect(w, r, "/login?error=用户名或密码错误", http.StatusSeeOther)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		http.Redirect(w, r, "/login?error=用户名或密码错误", http.StatusSeeOther)
		return
	}

	resetLoginAttempts(ip)

	token := createSession(user.ID, user.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func RegisterPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if username == "" || password == "" {
		http.Redirect(w, r, "/register?error=请填写所有字段", http.StatusSeeOther)
		return
	}

	if len(username) < 2 || len(username) > 20 {
		http.Redirect(w, r, "/register?error=用户名需要2-20个字符", http.StatusSeeOther)
		return
	}

	if len(password) < 8 {
		http.Redirect(w, r, "/register?error=密码至少8个字符", http.StatusSeeOther)
		return
	}

	if password != confirm {
		http.Redirect(w, r, "/register?error=两次密码不一致", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/register?error=注册失败，请重试", http.StatusSeeOther)
		return
	}

	user, err := db.CreateUser(username, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Redirect(w, r, "/register?error=用户名已存在", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/register?error=注册失败", http.StatusSeeOther)
		}
		return
	}

	token := createSession(user.ID, user.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func LogoutPost(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		db.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func PasswordPage(w http.ResponseWriter, r *http.Request) {
	data := injectUser(r, map[string]interface{}{
		"Error":   r.URL.Query().Get("error"),
		"Success": r.URL.Query().Get("success"),
	})

	tmpl, err := parseTemplates("layout.html", "password.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		return
	}

	tmpl.ExecuteTemplate(w, "layout", data)
}

func PasswordPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings/password", http.StatusSeeOther)
		return
	}

	userID := GetUserID(r)
	if userID == 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	oldPassword := r.FormValue("old_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if oldPassword == "" || newPassword == "" || confirmPassword == "" {
		http.Redirect(w, r, "/settings/password?error=请填写所有字段", http.StatusSeeOther)
		return
	}

	if len(newPassword) < 8 {
		http.Redirect(w, r, "/settings/password?error=新密码至少8个字符", http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		http.Redirect(w, r, "/settings/password?error=两次新密码不一致", http.StatusSeeOther)
		return
	}

	user, err := db.GetUser(userID)
	if err != nil {
		http.Redirect(w, r, "/settings/password?error=用户不存在", http.StatusSeeOther)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		http.Redirect(w, r, "/settings/password?error=旧密码错误", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Redirect(w, r, "/settings/password?error=修改失败，请重试", http.StatusSeeOther)
		return
	}

	if err := db.UpdateUserPassword(userID, string(hash)); err != nil {
		http.Redirect(w, r, "/settings/password?error=修改失败", http.StatusSeeOther)
		return
	}

	// Clear all other sessions for this user for security
	db.DeleteUserSessions(userID)

	// Re-create current session
	cookie, _ := r.Cookie("session")
	if cookie != nil {
		db.DeleteSession(cookie.Value)
	}
	token := createSession(userID, user.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30,
	})

	http.Redirect(w, r, "/settings/password?success=密码修改成功", http.StatusSeeOther)
}

func parseTemplatesCached(names ...string) (*template.Template, error) {
	return parseTemplates(names...)
}
