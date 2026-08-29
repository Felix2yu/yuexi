package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"yuexi/internal/db"
)

// TestMain spins up an isolated SQLite database for the handler package tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yuexi-handler-test")
	if err != nil {
		panic(err)
	}
	db.Init(filepath.Join(dir, "test.db"))
	code := m.Run()
	db.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

var userSeq int64

func uniqueName(prefix string) string {
	n := atomic.AddInt64(&userSeq, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), n)
}

// authedUser creates a user and a valid session, returning its id and the cookie.
func authedUser(t *testing.T) (int64, *http.Cookie) {
	t.Helper()
	u, err := db.CreateUser(uniqueName("u"), "hash")
	if err != nil {
		t.Fatal(err)
	}
	token := createSession(u.ID, u.Username)
	return u.ID, &http.Cookie{Name: "session", Value: token}
}

// authedReq builds a request carrying a valid session cookie.
func authedReq(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	_, c := authedUser(t)
	req := httptest.NewRequest(method, target, body)
	req.AddCookie(c)
	return req
}

// reqWithCookie builds a request carrying an explicit session cookie.
func reqWithCookie(method, target string, body io.Reader, c *http.Cookie) *http.Request {
	req := httptest.NewRequest(method, target, body)
	if c != nil {
		req.AddCookie(c)
	}
	return req
}

func do(h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func formBody(values map[string]string) *strings.Reader {
	v := url.Values{}
	for k, val := range values {
		v.Set(k, val)
	}
	return strings.NewReader(v.Encode())
}

// ----------------------------------------------------------------------------
// Pure helpers
// ----------------------------------------------------------------------------

func TestAvgMinMaxStdDev(t *testing.T) {
	vals := []float64{28, 30, 26, 29}
	if got := avg(vals); got != 28.3 {
		t.Errorf("avg = %v, want 28.3", got)
	}
	if got := avg(nil); got != 0 {
		t.Errorf("avg(nil) = %v, want 0", got)
	}
	if got := minVal(vals); got != 26 {
		t.Errorf("minVal = %v, want 26", got)
	}
	if got := maxVal(vals); got != 30 {
		t.Errorf("maxVal = %v, want 30", got)
	}
	sd := stdDev(vals)
	if sd <= 0 {
		t.Errorf("stdDev = %v, want > 0", sd)
	}
	if got := stdDev(nil); got != 0 {
		t.Errorf("stdDev(nil) = %v, want 0", got)
	}
}

func TestGetYearMonth(t *testing.T) {
	now := time.Now()
	req := httptest.NewRequest("GET", "/", nil)
	y, m := getYearMonth(req)
	if y != now.Year() || m != int(now.Month()) {
		t.Errorf("default = (%d,%d), want (%d,%d)", y, m, now.Year(), int(now.Month()))
	}

	req = httptest.NewRequest("GET", "/?year=2024&month=3", nil)
	y, m = getYearMonth(req)
	if y != 2024 || m != 3 {
		t.Errorf("parsed = (%d,%d), want (2024,3)", y, m)
	}

	// out-of-range month is ignored (falls back to current)
	req = httptest.NewRequest("GET", "/?month=13", nil)
	_, m = getYearMonth(req)
	if m != int(now.Month()) {
		t.Errorf("month=13 should fall back to %d, got %d", int(now.Month()), m)
	}
}

func TestCheckLoginRateLimit(t *testing.T) {
	ip := "203.0.113.10"
	resetLoginAttempts(ip)

	if ok, ra := checkLoginRateLimit(ip); !ok || ra != 0 {
		t.Fatalf("first attempt: ok=%v retryAfter=%d, want true/0", ok, ra)
	}
	// attempts 2..4 still allowed
	for i := 0; i < 3; i++ {
		if ok, _ := checkLoginRateLimit(ip); !ok {
			t.Fatalf("attempt should still be allowed before 5th")
		}
	}
	// 5th attempt triggers block
	if ok, ra := checkLoginRateLimit(ip); ok || ra != 900 {
		t.Fatalf("5th attempt: ok=%v retryAfter=%d, want false/900", ok, ra)
	}
	// while blocked, further attempts are denied with positive retryAfter
	if ok, ra := checkLoginRateLimit(ip); ok || ra <= 0 {
		t.Fatalf("blocked attempt: ok=%v retryAfter=%d, want false/>0", ok, ra)
	}
	resetLoginAttempts(ip)
	if ok, _ := checkLoginRateLimit(ip); !ok {
		t.Fatalf("after reset, attempt should be allowed")
	}
}

func TestParseTemplatesCached(t *testing.T) {
	tmpl, err := parseTemplatesCached("layout.html", "login.html")
	if err != nil {
		t.Fatalf("parseTemplatesCached error: %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}
}

func TestParseTemplatesError(t *testing.T) {
	if _, err := parseTemplates("layout.html", "nonexistent.html"); err == nil {
		t.Error("expected error for missing template")
	}
}

// ----------------------------------------------------------------------------
// Auth / session
// ----------------------------------------------------------------------------

func TestCreateSession(t *testing.T) {
	u, _ := db.CreateUser(uniqueName("cs"), "h")
	tok := createSession(u.ID, u.Username)
	if len(tok) != 64 {
		t.Errorf("token length = %d, want 64", len(tok))
	}
	s, err := db.GetSession(tok)
	if err != nil || s == nil {
		t.Fatalf("created session not retrievable: err=%v sess=%v", err, s)
	}
}

func TestGetSession(t *testing.T) {
	// no cookie
	if s := getSession(httptest.NewRequest("GET", "/", nil)); s != nil {
		t.Error("expected nil without cookie")
	}
	// invalid cookie value
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "nope"})
	if s := getSession(r); s != nil {
		t.Error("expected nil for invalid token")
	}
	// valid
	u, _ := db.CreateUser(uniqueName("gs"), "h")
	tok := createSession(u.ID, u.Username)
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.AddCookie(&http.Cookie{Name: "session", Value: tok})
	if s := getSession(r2); s == nil || s.UserID != u.ID {
		t.Error("expected valid session")
	}
}

func TestAuthMiddleware(t *testing.T) {
	passed := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { passed = true })

	// without session -> redirect, next not called
	rr := httptest.NewRecorder()
	AuthMiddleware(next).ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if passed || rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
		t.Errorf("unauthenticated: passed=%v code=%d loc=%q", passed, rr.Code, rr.Header().Get("Location"))
	}

	// with session -> next called
	passed = false
	_, c := authedUser(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	rr = httptest.NewRecorder()
	AuthMiddleware(next).ServeHTTP(rr, req)
	if !passed {
		t.Error("authenticated request should reach next")
	}
}

func TestInjectUser(t *testing.T) {
	// nil data, no session -> new map without CurrentUser
	out := injectUser(httptest.NewRequest("GET", "/", nil), nil)
	if _, ok := out["CurrentUser"]; ok {
		t.Error("no CurrentUser expected without session")
	}
	// with session -> CurrentUser injected
	_, c := authedUser(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	out = injectUser(req, map[string]interface{}{"Foo": 1})
	if out["Foo"] != 1 {
		t.Error("existing data should be preserved")
	}
	if out["CurrentUser"] == nil {
		t.Error("CurrentUser should be injected with session")
	}
}

func TestGetUserID(t *testing.T) {
	if id := GetUserID(httptest.NewRequest("GET", "/", nil)); id != 0 {
		t.Errorf("no session id = %d, want 0", id)
	}
	uid, c := authedUser(t)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	if id := GetUserID(req); id != uid {
		t.Errorf("id = %d, want %d", id, uid)
	}
}

func TestLoginPage(t *testing.T) {
	// not logged in -> renders
	rr := do(LoginPage, httptest.NewRequest("GET", "/login", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rr.Code)
	}
	// logged in -> redirect home
	rr = do(LoginPage, authedReq(t, "GET", "/login", nil))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Errorf("code=%d loc=%q, want 303 /", rr.Code, rr.Header().Get("Location"))
	}
}

func TestRegisterPage(t *testing.T) {
	rr := do(RegisterPage, httptest.NewRequest("GET", "/register", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rr.Code)
	}
	rr = do(RegisterPage, authedReq(t, "GET", "/register", nil))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Errorf("logged-in register should redirect to /")
	}
}

func TestLoginPost(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	user, err := db.CreateUser(uniqueName("login"), string(hash))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		ip     string
		method string
		form   map[string]string
		want   string // substring of redirect location
	}{
		{"get-method", "198.51.100.1", "GET", nil, "/login"},
		{"empty-fields", "198.51.100.2", "POST", map[string]string{}, "error"},
		{"user-not-found", "198.51.100.3", "POST", map[string]string{"username": "ghost", "password": "x"}, "error"},
		{"wrong-password", "198.51.100.4", "POST", map[string]string{"username": user.Username, "password": "wrong"}, "error"},
		{"success", "198.51.100.5", "POST", map[string]string{"username": user.Username, "password": "secret123"}, "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetLoginAttempts(tc.ip)
			var body io.Reader
			if tc.form != nil {
				body = formBody(tc.form)
			}
			req := httptest.NewRequest(tc.method, "/login", body)
			if tc.method == "POST" {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			req.RemoteAddr = tc.ip + ":1234"
			rr := do(LoginPost, req)
			loc := rr.Header().Get("Location")
			if !strings.Contains(loc, tc.want) {
				t.Errorf("%s: loc=%q, want contains %q", tc.name, loc, tc.want)
			}
			if tc.name == "success" {
				found := false
				for _, ck := range rr.Result().Cookies() {
					if ck.Name == "session" && ck.Value != "" {
						found = true
					}
				}
				if !found {
					t.Errorf("success should set session cookie")
				}
			}
		})
	}
}

func TestRegisterPost(t *testing.T) {
	// create a colliding user up front so the duplicate case can fail
	if _, err := db.CreateUser("dupuser", "hash"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		method string
		form   map[string]string
		want   string
	}{
		{"get-method", "GET", nil, "/register"},
		{"empty", "POST", map[string]string{}, "error"},
		{"short-username", "POST", map[string]string{"username": "a", "password": "longpass1", "confirm": "longpass1"}, "error"},
		{"short-password", "POST", map[string]string{"username": "alice", "password": "short", "confirm": "short"}, "error"},
		{"mismatch", "POST", map[string]string{"username": "alice", "password": "longpass1", "confirm": "longpass2"}, "error"},
		{"duplicate", "POST", map[string]string{"username": "dupuser", "password": "longpass1", "confirm": "longpass1"}, "error"},
		{"success", "POST", map[string]string{"username": uniqueName("reg"), "password": "longpass1", "confirm": "longpass1"}, "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.form != nil {
				body = formBody(tc.form)
			}
			req := httptest.NewRequest(tc.method, "/register", body)
			if tc.method == "POST" {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			rr := do(RegisterPost, req)
			loc := rr.Header().Get("Location")
			if !strings.Contains(loc, tc.want) {
				t.Errorf("%s: loc=%q, want contains %q", tc.name, loc, tc.want)
			}
		})
	}
	// pre-create a user so the duplicate case has something to collide with
	db.CreateUser("dupuser", "hash")
}

func TestLogoutPost(t *testing.T) {
	// with valid session: deletes session and clears cookie
	_, c := authedUser(t)
	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(c)
	rr := do(LogoutPost, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
		t.Errorf("code=%d loc=%q, want 303 /login", rr.Code, rr.Header().Get("Location"))
	}
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == "session" && ck.MaxAge != -1 {
			t.Errorf("session cookie should be cleared")
		}
	}
	// without session: still redirects
	rr = do(LogoutPost, httptest.NewRequest("POST", "/logout", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("no-session logout code = %d, want 303", rr.Code)
	}
}

func TestPasswordPage(t *testing.T) {
	rr := do(PasswordPage, authedReq(t, "GET", "/settings/password", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rr.Code)
	}
}

func TestPasswordPost(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpass12"), bcrypt.DefaultCost)
	user, _ := db.CreateUser(uniqueName("pw"), string(hash))

	cases := []struct {
		name string
		uid  int64
		form map[string]string
		want string
	}{
		{"get-method", user.ID, nil, "/settings/password"},
		{"no-session", 0, map[string]string{"old_password": "x", "new_password": "newpass12", "confirm_password": "newpass12"}, "/login"},
		{"empty", user.ID, map[string]string{}, "error"},
		{"short-new", user.ID, map[string]string{"old_password": "oldpass12", "new_password": "short", "confirm_password": "short"}, "error"},
		{"mismatch", user.ID, map[string]string{"old_password": "oldpass12", "new_password": "newpass12", "confirm_password": "other12"}, "error"},
		{"bad-old", user.ID, map[string]string{"old_password": "wrongold1", "new_password": "newpass12", "confirm_password": "newpass12"}, "error"},
		{"gone-user", 999999999, map[string]string{"old_password": "x", "new_password": "newpass12", "confirm_password": "newpass12"}, "error"},
		{"success", user.ID, map[string]string{"old_password": "oldpass12", "new_password": "newpass12", "confirm_password": "newpass12"}, "success"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.form != nil {
				body = formBody(tc.form)
			}
			req := httptest.NewRequest("POST", "/settings/password", body)
			if tc.form != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			if tc.uid != 0 {
				tok := createSession(tc.uid, "x")
				req.AddCookie(&http.Cookie{Name: "session", Value: tok})
			}
			rr := do(PasswordPost, req)
			if !strings.Contains(rr.Header().Get("Location"), tc.want) {
				t.Errorf("%s: loc=%q, want contains %q", tc.name, rr.Header().Get("Location"), tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Home / person
// ----------------------------------------------------------------------------

func TestHome(t *testing.T) {
	// no person -> redirect to /person
	rr := do(Home, authedReq(t, "GET", "/", nil))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/person" {
		t.Errorf("no-person code=%d loc=%q, want 303 /person", rr.Code, rr.Header().Get("Location"))
	}
	// with person -> renders
	uid, c := authedUser(t)
	db.CreatePerson(uid, "home-person", 28, 5)
	rr = do(Home, reqWithCookie("GET", "/", nil, c))
	if rr.Code != http.StatusOK {
		t.Errorf("with-person code = %d, want 200", rr.Code)
	}
}

func TestPersonList(t *testing.T) {
	rr := do(PersonList, authedReq(t, "GET", "/person", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rr.Code)
	}
}

func TestPersonCreate(t *testing.T) {
	// non-POST
	rr := do(PersonCreate, authedReq(t, "GET", "/person/create", nil))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/person" {
		t.Errorf("GET should redirect to /person")
	}
	// empty name
	req := authedReq(t, "POST", "/person/create", formBody(map[string]string{"name": ""}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(PersonCreate, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("empty name should redirect")
	}
	// success
	uid, c := authedUser(t)
	req = reqWithCookie("POST", "/person/create", formBody(map[string]string{"name": "新对象", "cycle_length": "30", "period_length": "6"}), c)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(PersonCreate, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Errorf("success code=%d loc=%q, want 303 /", rr.Code, rr.Header().Get("Location"))
	}
	// verify record created
	persons, _ := db.GetPersonsByUser(uid)
	if len(persons) != 1 {
		t.Errorf("person not created, got %d", len(persons))
	}
}

func TestPersonEdit(t *testing.T) {
	uid, _ := authedUser(t)
	p, _ := db.CreatePerson(uid, "edit-p", 28, 5)

	// GET with invalid id
	rr := do(PersonEdit, authedReq(t, "GET", "/person/edit?id=abc", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("invalid id GET should redirect")
	}
	// GET with non-existent person
	rr = do(PersonEdit, authedReq(t, "GET", "/person/edit?id=999999999", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("missing person GET should redirect")
	}
	// GET valid
	rr = do(PersonEdit, authedReq(t, "GET", "/person/edit?id="+itoa(p.ID), nil))
	if rr.Code != http.StatusOK {
		t.Errorf("valid GET code = %d, want 200", rr.Code)
	}
	// POST invalid id
	req := authedReq(t, "POST", "/person/edit?id=abc", formBody(map[string]string{"name": "x"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(PersonEdit, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("POST invalid id should redirect")
	}
	// POST empty name (no update)
	req = authedReq(t, "POST", "/person/edit?id="+itoa(p.ID), formBody(map[string]string{"name": ""}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(PersonEdit, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Errorf("POST empty name code=%d loc=%q", rr.Code, rr.Header().Get("Location"))
	}
	// POST valid update
	req = authedReq(t, "POST", "/person/edit?id="+itoa(p.ID), formBody(map[string]string{"name": "改名", "cycle_length": "31", "period_length": "7"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(PersonEdit, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Errorf("POST valid code=%d loc=%q", rr.Code, rr.Header().Get("Location"))
	}
	updated, _ := db.GetPerson(p.ID)
	if updated.Name != "改名" || updated.CycleLength != 31 {
		t.Errorf("person not updated: %+v", updated)
	}
}

func TestPersonDelete(t *testing.T) {
	uid, _ := authedUser(t)
	p, _ := db.CreatePerson(uid, "del-p", 28, 5)
	// invalid id -> redirect, no delete
	req := authedReq(t, "POST", "/person/delete", formBody(map[string]string{"id": "abc"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := do(PersonDelete, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("invalid id should redirect")
	}
	// valid delete
	req = authedReq(t, "POST", "/person/delete", formBody(map[string]string{"id": itoa(p.ID)}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(PersonDelete, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Errorf("delete code=%d loc=%q", rr.Code, rr.Header().Get("Location"))
	}
	if _, err := db.GetPerson(p.ID); err == nil {
		t.Error("person should be deleted")
	}
}

// ----------------------------------------------------------------------------
// Records
// ----------------------------------------------------------------------------

func TestRecordAPI(t *testing.T) {
	uid, c := authedUser(t)
	p, _ := db.CreatePerson(uid, "rec-p", 28, 5)
	db.CreateRecord(p.ID, "2024-03-01", nil, "note")

	rr := do(RecordAPI, reqWithCookie("GET", "/api/record?year=2024&month=3", nil, c))
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rr.Code)
	}
	var out struct {
		Records []map[string]interface{} `json:"records"`
		Periods []interface{}            `json:"periods"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(out.Records) != 1 {
		t.Errorf("records len = %d, want 1", len(out.Records))
	}
	// effective_end_date should be computed from period_length (no end_date set)
	ed := out.Records[0]["effective_end_date"]
	if ed != "2024-03-05" {
		t.Errorf("effective_end_date = %v, want 2024-03-05", ed)
	}
}

func TestRecordCreate(t *testing.T) {
	// non-POST -> 405
	rr := do(RecordCreate, authedReq(t, "GET", "/record/create", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET code = %d, want 405", rr.Code)
	}
	// missing fields -> redirect to referer
	req := authedReq(t, "POST", "/record/create", formBody(map[string]string{"person_id": "0", "start_date": ""}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(RecordCreate, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("missing fields should redirect")
	}
	// success
	uid, _ := authedUser(t)
	p, _ := db.CreatePerson(uid, "rc-p", 28, 5)
	req = authedReq(t, "POST", "/record/create", formBody(map[string]string{"person_id": itoa(p.ID), "start_date": "2024-04-01", "note": "hi"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(RecordCreate, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("success code = %d, want 303", rr.Code)
	}
	recs, _ := db.GetRecordsByPerson(p.ID)
	if len(recs) != 1 {
		t.Errorf("record not created, got %d", len(recs))
	}
}

func TestRecordEdit(t *testing.T) {
	// non-POST -> 405
	rr := do(RecordEdit, authedReq(t, "GET", "/record/edit", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET code = %d, want 405", rr.Code)
	}
	// missing id -> redirect
	req := authedReq(t, "POST", "/record/edit", formBody(map[string]string{"id": "0", "start_date": ""}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(RecordEdit, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("missing id should redirect")
	}
	// success
	uid, _ := authedUser(t)
	p, _ := db.CreatePerson(uid, "re-p", 28, 5)
	rec, _ := db.CreateRecord(p.ID, "2024-05-01", nil, "old")
	req = authedReq(t, "POST", "/record/edit", formBody(map[string]string{"id": itoa(rec.ID), "start_date": "2024-05-02", "note": "new"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(RecordEdit, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("success code = %d, want 303", rr.Code)
	}
	got, _ := db.GetRecordsByPerson(p.ID)
	if got[0].StartDate != "2024-05-02" {
		t.Errorf("record not edited: %+v", got[0])
	}
}

func TestRecordDelete(t *testing.T) {
	uid, _ := authedUser(t)
	p, _ := db.CreatePerson(uid, "rd-p", 28, 5)
	rec, _ := db.CreateRecord(p.ID, "2024-06-01", nil, "x")
	// id 0 -> no delete, redirect
	req := authedReq(t, "POST", "/record/delete", formBody(map[string]string{"id": "0"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := do(RecordDelete, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("id 0 should redirect")
	}
	if recs, _ := db.GetRecordsByPerson(p.ID); len(recs) != 1 {
		t.Errorf("record should still exist, got %d", len(recs))
	}
	// valid delete
	req = authedReq(t, "POST", "/record/delete", formBody(map[string]string{"id": itoa(rec.ID)}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(RecordDelete, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("delete code = %d, want 303", rr.Code)
	}
	if recs, _ := db.GetRecordsByPerson(p.ID); len(recs) != 0 {
		t.Errorf("record should be deleted, got %d", len(recs))
	}
}

// ----------------------------------------------------------------------------
// Stats
// ----------------------------------------------------------------------------

func TestStatsPage(t *testing.T) {
	rr := do(StatsPage, authedReq(t, "GET", "/stats", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("no-person should redirect to /person")
	}
	uid, c := authedUser(t)
	db.CreatePerson(uid, "st-p", 28, 5)
	rr = do(StatsPage, reqWithCookie("GET", "/stats", nil, c))
	if rr.Code != http.StatusOK {
		t.Errorf("with-person code = %d, want 200", rr.Code)
	}
}

func TestStatsAPI(t *testing.T) {
	uid, c := authedUser(t)
	p, _ := db.CreatePerson(uid, "st-p2", 28, 5)
	db.CreateRecord(p.ID, "2024-01-01", nil, "")

	rr := do(StatsAPI, reqWithCookie("GET", "/api/stats", nil, c))
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rr.Code)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := out[itoa(p.ID)]; !ok {
		t.Errorf("expected stats for person %d", p.ID)
	}
}

func TestDailyLogAPI(t *testing.T) {
	uid, _ := authedUser(t)
	p, _ := db.CreatePerson(uid, "dl-p", 28, 5)

	// GET all (person_id=0)
	rr := do(DailyLogAPI, authedReq(t, "GET", "/api/daily-log", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("GET all code = %d, want 200", rr.Code)
	}
	// GET by person
	rr = do(DailyLogAPI, authedReq(t, "GET", "/api/daily-log?person_id="+itoa(p.ID), nil))
	if rr.Code != http.StatusOK {
		t.Errorf("GET by person code = %d, want 200", rr.Code)
	}
	// POST invalid json
	req := authedReq(t, "POST", "/api/daily-log?person_id="+itoa(p.ID), strings.NewReader("not-json"))
	rr = do(DailyLogAPI, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid json code = %d, want 400", rr.Code)
	}
	// POST missing fields
	req = authedReq(t, "POST", "/api/daily-log", strings.NewReader(`{"date":"2024-01-01"}`))
	rr = do(DailyLogAPI, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing person_id code = %d, want 400", rr.Code)
	}
	// POST success
	body := `{"date":"2024-05-01","flow_level":2,"symptoms":"cramp","note":"x","weight":55.5,"temperature":36.6}`
	req = authedReq(t, "POST", "/api/daily-log?person_id="+itoa(p.ID), strings.NewReader(body))
	rr = do(DailyLogAPI, req)
	if rr.Code != http.StatusOK {
		t.Errorf("POST success code = %d, want 200", rr.Code)
	}
	// DELETE missing params
	req = authedReq(t, "DELETE", "/api/daily-log", nil)
	rr = do(DailyLogAPI, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("DELETE missing params code = %d, want 400", rr.Code)
	}
	// DELETE success
	req = authedReq(t, "DELETE", "/api/daily-log?person_id="+itoa(p.ID)+"&date=2024-05-01", nil)
	rr = do(DailyLogAPI, req)
	if rr.Code != http.StatusOK {
		t.Errorf("DELETE success code = %d, want 200", rr.Code)
	}
}

// ----------------------------------------------------------------------------
// Notify
// ----------------------------------------------------------------------------

func TestNotificationConfigAPI(t *testing.T) {
	// GET
	rr := do(NotificationConfigAPI, authedReq(t, "GET", "/api/notify/config", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("GET code = %d, want 200", rr.Code)
	}
	// POST invalid json
	req := authedReq(t, "POST", "/api/notify/config", strings.NewReader("bad"))
	rr = do(NotificationConfigAPI, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("POST invalid json code = %d, want 400", rr.Code)
	}
	// POST success
	req = authedReq(t, "POST", "/api/notify/config", strings.NewReader(`{"enabled":true,"shoutrrr_url":"logger://"}`))
	rr = do(NotificationConfigAPI, req)
	if rr.Code != http.StatusOK {
		t.Errorf("POST success code = %d, want 200", rr.Code)
	}
}

func TestNotificationTest(t *testing.T) {
	// non-POST
	rr := do(NotificationTest, authedReq(t, "GET", "/api/notify/test", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET code = %d, want 405", rr.Code)
	}
	// empty url
	req := authedReq(t, "POST", "/api/notify/test", formBody(map[string]string{"url": ""}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(NotificationTest, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty url code = %d, want 400", rr.Code)
	}
	// success (logger sink needs no network)
	req = authedReq(t, "POST", "/api/notify/test", formBody(map[string]string{"url": "logger://"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(NotificationTest, req)
	if rr.Code != http.StatusOK {
		t.Errorf("success code = %d, want 200 (logger sink)", rr.Code)
	}
	// failure
	req = authedReq(t, "POST", "/api/notify/test", formBody(map[string]string{"url": "::not-a-url"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(NotificationTest, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("invalid url code = %d, want 500", rr.Code)
	}
}

func TestNotificationStatus(t *testing.T) {
	uid, c := authedUser(t)
	p, _ := db.CreatePerson(uid, "ns-p", 28, 5)
	db.CreateRecord(p.ID, "2024-01-01", nil, "")

	rr := do(NotificationStatus, reqWithCookie("GET", "/api/notify/status", nil, c))
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rr.Code)
	}
	var out struct {
		Config   map[string]interface{} `json:"config"`
		Upcoming []interface{}          `json:"upcoming"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if out.Config == nil {
		t.Error("config should be present")
	}
}

func TestCycleAnomalyAPI(t *testing.T) {
	// no person -> empty array
	rr := do(CycleAnomalyAPI, authedReq(t, "GET", "/api/anomaly", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "[]") {
		t.Errorf("expected empty array, got %s", rr.Body.String())
	}
	// person belonging to another user -> empty
	uid, _ := authedUser(t)
	p, _ := db.CreatePerson(uid, "ca-p", 28, 5)
	other, _ := db.CreateUser(uniqueName("other"), "h")
	req := httptest.NewRequest("GET", "/api/anomaly?person_id="+itoa(p.ID), nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: createSession(other.ID, other.Username)})
	rr = do(CycleAnomalyAPI, req)
	if !strings.Contains(rr.Body.String(), "[]") {
		t.Errorf("cross-user person should yield empty array, got %s", rr.Body.String())
	}
	// valid person (same user who owns p)
	rr = do(CycleAnomalyAPI, reqWithCookie("GET", "/api/anomaly?person_id="+itoa(p.ID), nil, &http.Cookie{Name: "session", Value: createSession(uid, "x")}))
	if rr.Code != http.StatusOK {
		t.Errorf("valid person code = %d", rr.Code)
	}
}

// ----------------------------------------------------------------------------
// Static / settings / export
// ----------------------------------------------------------------------------

func TestHealth(t *testing.T) {
	rr := do(Health, httptest.NewRequest("GET", "/health", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "ok" {
		t.Errorf("health code=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestServeManifest(t *testing.T) {
	rr := do(ServeManifest, httptest.NewRequest("GET", "/manifest.json", nil))
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/manifest+json" {
		t.Errorf("manifest code=%d ct=%q", rr.Code, rr.Header().Get("Content-Type"))
	}
}

func TestServeSW(t *testing.T) {
	rr := do(ServeSW, httptest.NewRequest("GET", "/sw.js", nil))
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/javascript" {
		t.Errorf("sw code=%d ct=%q", rr.Code, rr.Header().Get("Content-Type"))
	}
}

func TestServeStatic(t *testing.T) {
	// local CSS is served with the correct content type and cache header
	rr := do(ServeStatic, httptest.NewRequest("GET", "/static/tailwind.css", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("tailwind.css code=%d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("tailwind.css content-type=%q", ct)
	}
	if !strings.Contains(rr.Header().Get("Cache-Control"), "max-age=86400") {
		t.Errorf("tailwind.css cache-control=%q", rr.Header().Get("Cache-Control"))
	}
	if rr.Body.Len() == 0 {
		t.Error("tailwind.css body should be non-empty")
	}

	// missing asset -> 404
	rr = do(ServeStatic, httptest.NewRequest("GET", "/static/does-not-exist.js", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing asset code=%d, want 404", rr.Code)
	}

	// path traversal is rejected
	rr = do(ServeStatic, httptest.NewRequest("GET", "/static/../sw.js", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("traversal code=%d, want 404", rr.Code)
	}
}

func TestServeFavicon(t *testing.T) {
	rr := do(ServeFavicon, httptest.NewRequest("GET", "/favicon.ico", nil))
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "image/") {
		t.Errorf("favicon code=%d ct=%q", rr.Code, rr.Header().Get("Content-Type"))
	}
	if rr.Body.Len() == 0 {
		t.Error("favicon body should be non-empty")
	}
}

func TestServeIcon(t *testing.T) {
	rr := httptest.NewRecorder()
	ServeIcon(rr, httptest.NewRequest("GET", "/icon/64", nil), 64)
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "image/png" {
		t.Errorf("icon code=%d ct=%q", rr.Code, rr.Header().Get("Content-Type"))
	}
}

func TestGenerateIcon(t *testing.T) {
	img := generateIcon(64)
	if img == nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		t.Errorf("generateIcon returned invalid image")
	}
}

func TestSettings(t *testing.T) {
	rr := do(Settings, authedReq(t, "GET", "/settings", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rr.Code)
	}
}

func TestExportPage(t *testing.T) {
	rr := do(ExportPage, authedReq(t, "GET", "/export", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rr.Code)
	}
}

func TestExportDownload(t *testing.T) {
	uid, c := authedUser(t)
	p, _ := db.CreatePerson(uid, "ex-p", 28, 5)
	db.CreateRecord(p.ID, "2024-01-01", nil, "")

	// all
	rr := do(ExportDownload, reqWithCookie("GET", "/export/download", nil, c))
	if rr.Code != http.StatusOK {
		t.Errorf("all export code = %d, want 200", rr.Code)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "yuexi_all.json") {
		t.Errorf("all export disposition = %q", cd)
	}
	// single person
	rr = do(ExportDownload, reqWithCookie("GET", "/export/download?person_id="+itoa(p.ID), nil, c))
	if rr.Code != http.StatusOK {
		t.Errorf("single export code = %d, want 200", rr.Code)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "yuexi_person_"+itoa(p.ID)+".json") {
		t.Errorf("single export disposition = %q", cd)
	}
}

func TestImportHandler(t *testing.T) {
	// non-POST -> redirect
	rr := do(ImportHandler, authedReq(t, "GET", "/import", nil))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/export" {
		t.Errorf("GET import loc=%q", rr.Header().Get("Location"))
	}
	// no file -> redirect with error
	req := authedReq(t, "POST", "/import", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = do(ImportHandler, req)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "error") {
		t.Errorf("no-file import loc=%q", rr.Header().Get("Location"))
	}
	// success: multipart upload of valid export JSON
	payload := `[{"person":{"name":"导入对象","cycle_length":28,"period_length":5},"records":[{"start_date":"2024-03-01","note":"x"}],"daily_logs":[]}]`
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "data.json")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(payload))
	mw.Close()

	_, c := authedUser(t)
	req = reqWithCookie("POST", "/import", &buf, c)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr = do(ImportHandler, req)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "success") {
		t.Errorf("import success loc=%q body=%q", rr.Header().Get("Location"), rr.Body.String())
	}
}

func itoa(i int64) string {
	return fmt.Sprintf("%d", i)
}
