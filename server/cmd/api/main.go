package main

import (
	"context"
	"log"
	"os"
	"time"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/httpapi"
	"four-check-management-system/server/internal/store"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := store.Open(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	svc := auth.NewService(db, db, []byte(mustEnv("JWT_SECRET")), 15*time.Minute, 7*24*time.Hour)
	r := httpapi.NewRouter(svc, os.Getenv("COOKIE_SECURE") == "true")
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("api listening on %s", addr)
	log.Fatal(r.Run(addr))
}
func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}
