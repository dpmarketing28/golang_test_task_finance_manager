// Command llmcheck sends one test chat-completion request using the LLM
// settings from config (LLM_API_KEY / LLM_BASE_URL / LLM_MODEL), to
// verify the configured provider/model/key actually work before wiring
// the NLU agent into the bot.
//
// Usage: go run ./cmd/llmcheck
package main

import (
	"context"
	"fmt"
	"log"

	"test_task_goblin/internal/config"
	"test_task_goblin/internal/llm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.LLMAPIKey == "" || cfg.LLMModel == "" {
		log.Fatal("llmcheck: set LLM_API_KEY and LLM_MODEL in .env first")
	}

	client := llm.New(llm.Config{
		APIKey:  cfg.LLMAPIKey,
		BaseURL: cfg.LLMBaseURL,
		Model:   cfg.LLMModel,
	})

	fmt.Printf("Запрос к модели %q через %s ...\n", cfg.LLMModel, cfg.LLMBaseURL)

	reply, err := client.Complete(context.Background(), []llm.Message{
		{Role: "user", Content: "Ответь одним коротким предложением: ты на связи?"},
	})
	if err != nil {
		log.Fatalf("llmcheck: %v", err)
	}

	fmt.Println("Ответ модели:", reply)
}
