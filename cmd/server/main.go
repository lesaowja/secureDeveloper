package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
	_ "modernc.org/sqlite"
)

const authorizationCookieName = "authorization"

type User struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"-"`
	Balance  int64  `json:"balance"`
	IsAdmin  bool   `json:"is_admin"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type WithdrawAccountRequest struct {
	Password string `json:"password"`
}

type UserResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Balance  int64  `json:"balance"`
	IsAdmin  bool   `json:"is_admin"`
}

type LoginResponse struct {
	AuthMode string       `json:"auth_mode"`
	Token    string       `json:"token"`
	User     UserResponse `json:"user"`
}

type PostView struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	OwnerID     uint   `json:"owner_id"`
	Author      string `json:"author"`
	AuthorEmail string `json:"author_email"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type CreatePostRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type UpdatePostRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type PostListResponse struct {
	Posts []PostView `json:"posts"`
}

type PostResponse struct {
	Post PostView `json:"post"`
}

type DepositRequest struct {
	Amount int64 `json:"amount"`
}

type BalanceWithdrawRequest struct {
	Amount int64 `json:"amount"`
}

type TransferRequest struct {
	ToUsername string `json:"to_username"`
	Amount     int64  `json:"amount"`
}

type Store struct {
	db *sql.DB
}

type SessionStore struct {
	tokens map[string]User
}

func main() {
	store, err := openStore("./app.db", "./schema.sql", "./seed.sql")
	if err != nil {
		panic(err)
	}
	defer store.close()

	sessions := newSessionStore()

	initLogger()
	router := gin.Default()
	registerStaticRoutes(router)
	router.Use(JSONLogger())

	auth := router.Group("/api/auth")
	{
		auth.POST("/register", func(c *gin.Context) {
			var request RegisterRequest
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid register request"})
				return
			}

			if err != nil {
				c.JSON(400, gin.H{"error": "유저 추가 실패"})
				return
			}

			s := store
			s.db.Exec(`INSERT INTO users (username, name, email, phone, password, balance, is_admin)VALUES (?, ?, ?, ?, ?, 0, 0);`, request.Username, request.Name, request.Email, request.Phone, request.Password)
			c.JSON(http.StatusAccepted, gin.H{

				"message": "회원가입에 성공하셨습니다.",
				"user": gin.H{
					"username": request.Username,
					"name":     request.Name,
					"email":    request.Email,
					"phone":    request.Phone,
					"password": request.Password,
				},
			})
		})

		auth.POST("/login", func(c *gin.Context) {
			var request LoginRequest
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid login request"})
				return
			}

			user, ok, err := store.findUserByUsername(request.Username)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to load user"})
				return
			}
			if !ok || user.Password != request.Password {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid credentials"})
				println(user.Password)
				println(request.Password)
				return
			}

			token, err := sessions.create(user)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to create session"})
				return
			}

			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie(authorizationCookieName, token, 60*60*8, "/", "", false, true)
			c.JSON(http.StatusOK, LoginResponse{
				AuthMode: "header-and-cookie",
				Token:    token,
				User:     makeUserResponse(user),
			})
		})

		auth.POST("/logout", func(c *gin.Context) {
			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			if _, ok := sessions.lookup(token); !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			sessions.delete(token)
			clearAuthorizationCookie(c)
			c.JSON(http.StatusOK, gin.H{
				"message": "dummy logout handler",
				"todo":    "replace with revoke or audit logic if needed",
			})
		})

		auth.POST("/withdraw", func(c *gin.Context) {
			var request WithdrawAccountRequest
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid withdraw request"})
				return
			}

			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			user, ok := sessions.lookup(token)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			if request.Password == user.Password {

				s := store
				s.db.Exec(`DELETE FROM users WHERE id =?;`, user.ID)
				c.JSON(http.StatusAccepted, gin.H{
					"message": "success",
					"todo":    "success to remove your data",
					"user":    makeUserResponse(user),
				})
			} else {
				c.JSON(http.StatusAccepted, gin.H{
					"message": "fail",
					"todo":    "fail to remove your data",
					"user":    makeUserResponse(user),
				})
			}

		})
	}

	protected := router.Group("/api")
	{

		s := store
		protected.GET("/me", func(c *gin.Context) {
			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			user, ok := sessions.lookup(token)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}
			if err != nil {
				log.Fatal(err)
			}
			user, ok, err = store.findUserByUsername(user.Username)
			c.JSON(http.StatusOK, gin.H{"user": makeUserResponse(user)})

		})
		//checkpoint
		protected.POST("/banking/deposit", func(c *gin.Context) {
			var request DepositRequest

			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid deposit request"})
				return
			}
			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			user, ok := sessions.lookup(token)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}
			if request.Amount > 0 {

				user, ok, err = store.findUserByUsername(user.Username)
				c.JSON(http.StatusOK, gin.H{
					"message": "성공적으로 입금이 완료되었습니다.",
					"user":    makeUserResponse(user),
					"balance": user.Balance,
				})
				s.db.Exec(`UPDATE users SET balance = balance + ? WHERE id = ?;`, request.Amount, user.ID)
			} else {
				c.JSON(http.StatusOK, gin.H{
					"message": "0원 보다 작은 입금은 불가능 합니다.",
					"user":    makeUserResponse(user),
					"amount":  request.Amount,
				})
			}

		})

		protected.POST("/banking/withdraw", func(c *gin.Context) {
			var request BalanceWithdrawRequest
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid withdraw request"})
				return
			}

			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			user, ok := sessions.lookup(token)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			if request.Amount > 0 {
				user, ok, err = store.findUserByUsername(user.Username)
				if user.Balance > request.Amount {
					c.JSON(http.StatusOK, gin.H{

						"message": "성공적으로 출금이 완료되었습니다.",
						"balance": user.Balance + request.Amount,
					})

					s.db.Exec(`UPDATE users SET balance = balance - ? WHERE id = ?;`, request.Amount, user.ID)
				} else {
					c.JSON(http.StatusOK, gin.H{

						"message": "송금 금액이 부족합니다.",
					})
				}
			} else {
				c.JSON(http.StatusOK, gin.H{
					"message": "0원 보다 작은 출금은 불가능 합니다.",
					"user":    makeUserResponse(user),
					"amount":  request.Amount,
				})
			}
		})

		protected.POST("/banking/transfer", func(c *gin.Context) {

			var request TransferRequest
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid transfer request"})
				return
			}

			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			user, ok := sessions.lookup(token)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			user, ok, err = store.findUserByUsername(user.Username)
			if user.Balance < request.Amount {
				c.JSON(http.StatusOK, gin.H{
					"message": "가진돈이 부족합니다.",
					"user":    makeUserResponse(user),
					"target":  request.ToUsername,
					"amount":  request.Amount,
				})
			} else {
				if request.Amount > 0 {
					c.JSON(http.StatusOK, gin.H{
						"message": "송금완료",
						"user":    makeUserResponse(user),
						"target":  request.ToUsername,
						"amount":  request.Amount,
					})
					s.db.Exec(`UPDATE users SET balance = balance + ? WHERE username = ?;`, request.Amount, request.ToUsername)
					s.db.Exec(`UPDATE users SET balance = balance - ? WHERE username = ?;`, request.Amount, user.Username)
				} else {
					c.JSON(http.StatusOK, gin.H{
						"message": "음수의 금액을 보낼수 없습니다.",
						"user":    makeUserResponse(user),
						"target":  request.ToUsername,
						"amount":  request.Amount,
					})
				}

			}

		})

		protected.GET("/posts", func(c *gin.Context) {
			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			if _, ok := sessions.lookup(token); !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}
			for i := 1; i <= 5; i++ {
				pos, _, _ := s.findposts(i)
				c.JSON(http.StatusOK, PostListResponse{
					Posts: []PostView{
						{
							ID:          pos.ID,
							Title:       pos.Title,
							Content:     pos.Content,
							OwnerID:     pos.OwnerID,
							Author:      pos.Author,
							AuthorEmail: pos.AuthorEmail,
							CreatedAt:   pos.CreatedAt,
							UpdatedAt:   pos.UpdatedAt,
						},
					},
				})

			}

		})

		protected.POST("/posts", func(c *gin.Context) {
			var request CreatePostRequest
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid create request"})
				return
			}

			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			user, ok := sessions.lookup(token)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			now := time.Now().Format(time.RFC3339)
			s.db.Exec(`INSERT INTO posts (title,content,owner_id,created_at,updated_at)VALUES(?,?,?,?,?);`, request.Title, request.Content, user.ID, now, now)

			c.JSON(http.StatusCreated, gin.H{
				"message": "dummy create post handler",
				"todo":    "replace with insert query",
				"post": PostView{
					ID:          1,
					Title:       strings.TrimSpace(request.Title),
					Content:     strings.TrimSpace(request.Content),
					OwnerID:     user.ID,
					Author:      user.Name,
					AuthorEmail: user.Email,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			})
		})

		protected.GET("/posts/:id", func(c *gin.Context) {
			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			if _, ok := sessions.lookup(token); !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			c.JSON(http.StatusOK, PostResponse{
				Post: PostView{
					ID:          1,
					Title:       "Dummy Post",
					Content:     "This is a fixed dummy response. Replace this later with real board logic.",
					OwnerID:     1,
					Author:      "Alice Admin",
					AuthorEmail: "alice.admin@example.com",
					CreatedAt:   "2026-03-19T09:00:00Z",
					UpdatedAt:   "2026-03-19T09:00:00Z",
				},
			})
		})

		protected.PUT("/posts/:id", func(c *gin.Context) {
			var request UpdatePostRequest
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "invalid update request"})
				return
			}

			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			user, ok := sessions.lookup(token)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			now := time.Now().Format(time.RFC3339)
			c.JSON(http.StatusOK, gin.H{
				"message": "dummy update post handler",
				"todo":    "replace with ownership check and update query",
				"post": PostView{
					ID:          1,
					Title:       strings.TrimSpace(request.Title),
					Content:     strings.TrimSpace(request.Content),
					OwnerID:     user.ID,
					Author:      user.Name,
					AuthorEmail: user.Email,
					CreatedAt:   "2026-03-19T09:00:00Z",
					UpdatedAt:   now,
				},
			})
		})

		protected.DELETE("/posts/:id", func(c *gin.Context) {
			token := tokenFromRequest(c)
			if token == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "missing authorization token"})
				return
			}
			if _, ok := sessions.lookup(token); !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid authorization token"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"message": "dummy delete post handler",
				"todo":    "replace with ownership check and delete query",
			})
		})
	}

	if err := router.Run(":8080"); err != nil {
		panic(err)
	}
}

func openStore(databasePath, schemaFile, seedFile string) (*Store, error) {
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.initialize(schemaFile, seedFile); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) close() error {
	return s.db.Close()
}

func (s *Store) initialize(schemaFile, seedFile string) error {
	if err := s.execSQLFile(schemaFile); err != nil {
		return err
	}
	if err := s.execSQLFile(seedFile); err != nil {
		return err
	}
	return nil
}

func (s *Store) execSQLFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(string(content))
	return err
}

func (s *Store) findUserByUsername(username string) (User, bool, error) {
	row := s.db.QueryRow(`
		SELECT id, username, name, email, phone, password, balance, is_admin
		FROM users
		WHERE username = ?
	`, strings.TrimSpace(username))

	var user User
	var isAdmin int64
	if err := row.Scan(&user.ID, &user.Username, &user.Name, &user.Email, &user.Phone, &user.Password, &user.Balance, &isAdmin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, false, nil
		}
		return User{}, false, err
	}
	user.IsAdmin = isAdmin == 1

	return user, true, nil
}

func (s *Store) findposts(id_ int) (PostView, bool, error) {
	row := s.db.QueryRow(`
		SELECT id,title,content,owner_id,created_at,updated_at,owner_id
		FROM posts
		WHERE id = ?
	`, id_)
	var post PostView
	if err := row.Scan(&post.ID, &post.Title, &post.Content, &post.OwnerID, &post.CreatedAt, &post.UpdatedAt, &post.OwnerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PostView{}, false, nil
		}
		return PostView{}, false, err
	}

	return post, true, nil
}

func newSessionStore() *SessionStore {
	return &SessionStore{
		tokens: make(map[string]User),
	}
}

func (s *SessionStore) create(user User) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}

	s.tokens[token] = user
	return token, nil
}

func (s *SessionStore) lookup(token string) (User, bool) {
	user, ok := s.tokens[token]
	return user, ok
}

func (s *SessionStore) delete(token string) {
	delete(s.tokens, token)
}

// fe 페이지 캐싱으로 테스트에 혼동이 있어, 별도 처리없이 main에 두시면 될 것 같습니다
// registerStaticRoutes 는 정적 파일(HTML, JS, CSS)을 제공하는 라우트를 등록한다.
func registerStaticRoutes(router *gin.Engine) {
	// 브라우저 캐시 비활성화 — 정적 파일과 루트 경로에만 적용
	router.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/") || c.Request.URL.Path == "/" {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	})
	router.Static("/static", "./static")
	router.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})
}

func makeUserResponse(user User) UserResponse {
	return UserResponse{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		Email:    user.Email,
		Phone:    user.Phone,
		Balance:  user.Balance,
		IsAdmin:  user.IsAdmin,
	}
}

func clearAuthorizationCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authorizationCookieName, "", -1, "/", "", false, true)
}

func tokenFromRequest(c *gin.Context) string {
	headerValue := strings.TrimSpace(c.GetHeader("Authorization"))
	if headerValue != "" {
		return headerValue
	}

	cookieValue, err := c.Cookie(authorizationCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookieValue)
}

func newSessionToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
func initLogger() {
	os.MkdirAll("logs", 0755)
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(&lumberjack.Logger{
		Filename:   "./logs/All.log",
		MaxSize:    1,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	})
}
func JSONLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.WithFields(log.Fields{
			"ip":     c.ClientIP(),
			"method": c.Request.Method,
			"path":   c.Request.URL.Path,
			"query":  c.Request.URL.RawQuery,
			"header": c.Request.Header,
			"body":   c.Request.Body,
		}).Info("incoming request")
		c.Next()
	}
}
