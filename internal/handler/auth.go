package handler

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"strings"
	"yuexi/internal/db"

	"golang.org/x/crypto/bcrypt"
)

var sessionKey = make([]byte, 32)

func init() {
	rand.Read(sessionKey)
}

type Session struct {
	UserID   int64
	Username string
}

var sessions = map[string]*Session{}

func createSession(userID int64, username string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	sessions[token] = &Session{UserID: userID, Username: username}
	return token
}

func getSession(r *http.Request) *Session {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	return sessions[cookie.Value]
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

	if len(password) < 4 {
		http.Redirect(w, r, "/register?error=密码至少4个字符", http.StatusSeeOther)
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
		delete(sessions, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func parseTemplatesCached(names ...string) (*template.Template, error) {
	return parseTemplates(names...)
}
