// Command app is the application entry point. It only wires dependencies
// together (config -> db -> repositories -> usecases -> transports) and
// runs them; it holds no business logic of its own.
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"test_task_goblin/internal/config"
	"test_task_goblin/internal/httpapi"
	"test_task_goblin/internal/storage/postgres"
	"test_task_goblin/internal/telegram"
	"test_task_goblin/internal/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	if err := postgres.Migrate(ctx, db); err != nil {
		log.Fatalf("postgres: migrate: %v", err)
	}

	userRepo := postgres.NewUserRepo(db)
	expenseRepo := postgres.NewExpenseRepo(db)

	userSvc := usecase.NewUserService(userRepo, cfg.FamilyAdminTelegramID)
	expenseSvc := usecase.NewExpenseService(expenseRepo)

	bot, err := telegram.New(cfg.TelegramBotToken, userSvc)
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: httpapi.NewRouter(expenseSvc, userSvc, userRepo),
	}

	go func() {
		log.Printf("httpapi: listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("httpapi: %v", err)
		}
	}()

	go func() {
		if err := bot.Run(); err != nil {
			log.Fatalf("telegram: run: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bot.Stop()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("httpapi: shutdown: %v", err)
	}
}
