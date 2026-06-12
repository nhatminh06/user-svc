package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ctx = context.Background()
	rdb *redis.Client
)

type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	rdb = redis.NewClient(&redis.Options{
		Addr: getEnv("REDIS_URL", "localhost:6379"),
	})

	// Test Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("WARNING: Redis not available: %v", err)
	} else {
		log.Println("Connected to Redis")
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/users", usersHandler)
	http.HandleFunc("/users/", userByIDHandler)

	port := getEnv("PORT", "8081")
	log.Printf("user-svc starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "user-svc",
	})
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPost:
		createUser(w, r)
	case http.MethodGet:
		listUsers(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func userByIDHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := strings.TrimPrefix(r.URL.Path, "/users/")
	if id == "" {
		http.Error(w, "missing user id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getUser(w, id)
	case http.MethodDelete:
		deleteUser(w, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if user.Username == "" || user.Email == "" {
		http.Error(w, "username and email required", http.StatusBadRequest)
		return
	}

	user.ID = fmt.Sprintf("user_%d", time.Now().UnixNano())
	user.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	data, _ := json.Marshal(user)
	rdb.Set(ctx, "user:"+user.ID, data, 0)
	rdb.SAdd(ctx, "users", user.ID)

	log.Printf("Created user: %s (%s)", user.Username, user.ID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func getUser(w http.ResponseWriter, id string) {
	data, err := rdb.Get(ctx, "user:"+id).Result()
	if err == redis.Nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(data))
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	ids, err := rdb.SMembers(ctx, "users").Result()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	users := []User{}
	for _, id := range ids {
		data, err := rdb.Get(ctx, "user:"+id).Result()
		if err != nil {
			continue
		}
		var user User
		json.Unmarshal([]byte(data), &user)
		users = append(users, user)
	}

	json.NewEncoder(w).Encode(users)
}

func deleteUser(w http.ResponseWriter, id string) {
	rdb.Del(ctx, "user:"+id)
	rdb.SRem(ctx, "users", id)
	log.Printf("Deleted user: %s", id)
	w.WriteHeader(http.StatusNoContent)
}
