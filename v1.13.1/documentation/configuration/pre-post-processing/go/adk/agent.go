package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

const systemPrompt = `You're a helpful hotel assistant. You handle hotel searching, booking and
cancellations. When the user searches for a hotel, mention it's name, id,
location and price tier. Always mention hotel ids while performing any
searches. This is very important for any operations. For any bookings or
cancellations, please provide the appropriate confirmation. Be sure to
update checkin or checkout dates if mentioned by the user.
Don't ask for confirmations from the user.`

var queries = []string{
	"Book hotel with id 3.",
	"Update my hotel with id 3 with checkin date 2025-01-04 and checkout date 2025-01-20",
}

// Pre-processing
func enforceBusinessRules(ctx tool.Context, tool tool.Tool, args map[string]any) (map[string]any, error) {

	fmt.Printf("POLICY CHECK: Intercepting '%s'\n", tool.Name())
	if tool.Name() == "update-hotel" {
		checkinStr, okCheckin := args["checkin_date"].(string)
		checkoutStr, okCheckout := args["checkout_date"].(string)

		if okCheckin && okCheckout {
			startDate, errStart := time.Parse("2006-01-02", checkinStr)
			endDate, errEnd := time.Parse("2006-01-02", checkoutStr)
			if errStart != nil || errEnd != nil {
				return nil, nil
			}

			duration := endDate.Sub(startDate).Hours() / 24
			if duration > 14 {
				fmt.Println("BLOCKED: Stay too long")
				return map[string]any{"Error": "Maximum stay duration is 14 days."}, nil
			}
		}
	}
	return nil, nil
}

// Post-processing
func enrichResponse(ctx tool.Context, tool tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
	resultStr := fmt.Sprintf("%v", result)

	if tool.Name() == "book-hotel" {
		if err != nil {
			return nil, err
		}
		if _, ok := result["Error"]; !ok && !strings.Contains(resultStr, "Error") {
			const loyaltyBonus = 500
			enrichedResult := fmt.Sprintf("Booking Confirmed!\n You earned %d Loyalty Points with this stay.\n\nSystem Details: %s", loyaltyBonus, resultStr)
			return map[string]any{"confirmation": enrichedResult}, nil
		}
	}
	return result, nil
}

func main() {
	genaiKey := os.Getenv("GOOGLE_API_KEY")
	toolboxURL := "http://localhost:5000"
	ctx := context.Background()

	toolboxClient, err := tbadk.NewToolboxClient(toolboxURL)
	if err != nil {
		log.Fatalf("Failed to create MCP Toolbox client: %v", err)
	}

	toolsetName := "my-toolset"
	mcpTools, err := toolboxClient.LoadToolset(toolsetName, ctx)
	if err != nil {
		log.Fatalf("Failed to load MCP toolset '%s': %v\nMake sure your Toolbox server is running.", toolsetName, err)
	}

	model, err := gemini.NewModel(ctx, "gemini-3-flash-preview", &genai.ClientConfig{
		APIKey: genaiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	tools := make([]tool.Tool, len(mcpTools))
	for i := range mcpTools {
		tools[i] = &mcpTools[i]
	}
	llmagent, err := llmagent.New(llmagent.Config{
		Name:        "hotel_assistant",
		Model:       model,
		Description: "Agent to answer questions about hotels.",
		Instruction: systemPrompt,
		Tools:       tools,
		// Add pre- and post- processing hooks
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{enforceBusinessRules},
		AfterToolCallbacks:  []llmagent.AfterToolCallback{enrichResponse},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	appName := "hotel_assistant"
	userID := "user-123"
	sessionService := session.InMemoryService()
	respSess, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		log.Fatalf("Failed to create the session service: %v", err)
	}
	sess := respSess.Session

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          llmagent,
		SessionService: sessionService,
	})
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	for i, query := range queries {
		fmt.Printf("\n=== Query %d: %s ===\n", i+1, query)
		userMsg := genai.NewContentFromText(query, genai.RoleUser)
		streamingMode := agent.StreamingModeSSE

		runIter := r.Run(ctx, userID, sess.ID(), userMsg, agent.RunConfig{
			StreamingMode: streamingMode,
		})

		fmt.Print("AI: ")
		for event := range runIter {
			if event != nil && event.Content != nil {
				for _, p := range event.Content.Parts {
					fmt.Print(p.Text)
				}
			}
		}

		fmt.Println("\n" + strings.Repeat("-", 80) + "\n")
	}
}
