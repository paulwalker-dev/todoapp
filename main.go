package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
)

func DBConnect(ctx context.Context) error {
	opts := &redis.Options{}

	addr, ok := os.LookupEnv("DB_ADDR")
	if !ok {
		return errors.New("DB_ADDR cannot be empty")
	}
	opts.Addr = addr

	name, ok := os.LookupEnv("DB_NAME")
	if ok {
		db, err := strconv.Atoi(name)
		if err != nil {
			return errors.New("DB_NAME needs to be a number")
		}
		opts.DB = db
	}

	user, ok := os.LookupEnv("DB_USER")
	if ok {
		opts.Username = user
	}

	slog.InfoContext(ctx, "Connecting to Redis",
		slog.Any("options", opts))

	pass, ok := os.LookupEnv("DB_PASS")
	if ok {
		opts.Password = pass
	}

	rdb = redis.NewClient(opts)
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to connect to Redis",
			slog.String("msg", err.Error()))
		return err
	}

	slog.InfoContext(ctx, "Connected to Redis",
		slog.String("ping", pong))

	return nil
}

var rdb *redis.Client

type Todo struct {
	ID   int64
	Body string
	Done bool
}

func GetTodo(ctx context.Context, id int64) Todo {
	result, err := rdb.Get(ctx, fmt.Sprintf("todos:%d", id)).Result()
	if err != nil {
		return Todo{ID: -1, Body: "ERROR: Unable to retrieve todo"}
	}

	done, err := rdb.SIsMember(ctx, "todos:done", fmt.Sprintf("todos:%d", id)).Result()
	if err != nil {
		done = false
	}

	return Todo{
		ID:   id,
		Body: result,
		Done: done,
	}
}

func GetAllTodos(ctx context.Context) []Todo {
	todoList, err := rdb.ZRevRangeWithScores(ctx, "todos:all", 0, -1).Result()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to retrieve todo list",
			slog.String("err", err.Error()))
		return []Todo{
			{ID: -1, Body: "ERROR: Unable to retrieve todo list from DB"},
		}
	}

	var result []Todo

	for _, todo := range todoList {
		id := int64(todo.Score)
		result = append(result, GetTodo(ctx, id))
	}

	return result
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	todos := GetAllTodos(r.Context())
	t, _ := template.ParseFiles("index.gohtml")
	t.Execute(w, todos)
}

func NewTodo(ctx context.Context, content string) int64 {
	id, err := rdb.Incr(ctx, "todos:counter").Result()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create todo",
			slog.String("err", err.Error()))
		return -1
	}

	if err := rdb.Set(ctx, fmt.Sprintf("todos:%d", id), content, 0).Err(); err != nil {
		slog.ErrorContext(ctx, "Failed to create todo",
			slog.String("err", err.Error()))
		return -1
	}

	if err := rdb.ZAdd(ctx, "todos:all", &redis.Z{
		Score:  float64(id),
		Member: fmt.Sprintf("todos:%d", id),
	}).Err(); err != nil {
		slog.ErrorContext(ctx, "Failed to create todo",
			slog.String("err", err.Error()))
		return -1
	}

	return id
}

func newHandler(w http.ResponseWriter, r *http.Request) {
	todo := r.FormValue("todo")
	NewTodo(r.Context(), todo)
	http.Redirect(w, r, "/todoapp", http.StatusFound)
}

func doneHandler(w http.ResponseWriter, r *http.Request) {
	rawID := r.URL.Path[len("/finish/"):]
	id, err := strconv.Atoi(rawID)
	if err != nil {
		slog.ErrorContext(r.Context(), "Invalid todo id",
			slog.Int("id", id))
		http.Redirect(w, r, "/todoapp", http.StatusNotFound)
		return
	}
	if err := rdb.SAdd(r.Context(), "todos:done", fmt.Sprintf("todos:%d", id)).Err(); err != nil {
		slog.ErrorContext(r.Context(), "Unable to finish todo",
			slog.String("err", err.Error()))
	}
	http.Redirect(w, r, "/todoapp", http.StatusFound)
}

func resetHandler(w http.ResponseWriter, r *http.Request) {
	rdb.FlushAll(r.Context())
	http.Redirect(w, r, "/todoapp", http.StatusFound)
}

func main() {
	ctx := context.Background()
	_ = godotenv.Load()

	err := DBConnect(ctx)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer rdb.Close()

	http.HandleFunc("/todoapp", indexHandler)
	http.HandleFunc("/new", newHandler)
	http.HandleFunc("/finish/", doneHandler)
	http.HandleFunc("/reset", resetHandler)
	log.Fatal(http.ListenAndServe(":8000", nil))
}
