package main

import (
	"context"
	"fmt"
	"log"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	"github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

const systemPrompt = `
You're a helpful hotel assistant. You handle hotel searching, booking, and
cancellations. When the user searches for a hotel, mention its name, id,
location and price tier. Always mention hotel ids while performing any
searches. This is very important for any operations. For any bookings or
cancellations, please provide the appropriate confirmation. Be sure to
update checkin or checkout dates if mentioned by the user.
Don't ask for confirmations from the user.
`

var queries = []string{
	"Find hotels in Basel with Basel in its name.",
	"Can you book the hotel Hilton Basel for me?",
	"Oh wait, this is too expensive. Please cancel it and book the Hyatt Regency instead.",
	"My check in dates would be from April 10, 2024 to April 19, 2024.",
}

func main() {
	ctx := context.Background()

	// Create Toolbox Client
	toolboxClient, err := core.NewToolboxClient("http://127.0.0.1:5000")
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Load the tools using the MCP Toolbox SDK.
	tools, err := toolboxClient.LoadToolset("my-toolset", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v\nMake sure your Toolbox server is running and the tool is configured.", err)
	}

	// Initialize Genkit
	g := genkit.Init(ctx,
		genkit.WithPlugins(&googlegenai.GoogleAI{}),
		genkit.WithDefaultModel("googleai/gemini-3.6-flash"),
	)
	if err != nil {
		log.Fatalf("Failed to init genkit: %v\n", err)
	}

	// Create a conversation history
	conversationHistory := []*ai.Message{
		ai.NewSystemTextMessage(systemPrompt),
	}

	// Convert your tool to a Genkit tool.
	genkitTools := make([]ai.Tool, len(tools))
	for i, tool := range tools {
		newTool, err := tbgenkit.ToGenkitTool(tool, g)
		if err != nil {
			log.Fatalf("Failed to convert tool: %v\n", err)
		}
		genkitTools[i] = newTool
	}

	toolRefs := make([]ai.ToolRef, len(genkitTools))

	for i, tool := range genkitTools {
		toolRefs[i] = tool
	}

	for _, query := range queries {
		conversationHistory = append(conversationHistory, ai.NewUserTextMessage(query))
		response, err := genkit.Generate(ctx, g,
			ai.WithMessages(conversationHistory...),
			ai.WithTools(toolRefs...),
			ai.WithReturnToolRequests(true),
		)

		if err != nil {
			log.Fatalf("%v\n", err)
		}
		conversationHistory = append(conversationHistory, response.Message)

		parts := []*ai.Part{}

		for _, req := range response.ToolRequests() {
			tool := genkit.LookupTool(g, req.Name)
			if tool == nil {
				log.Fatalf("tool %q not found", req.Name)
			}

			output, err := tool.RunRaw(ctx, req.Input)
			if err != nil {
				log.Fatalf("tool %q execution failed: %v", tool.Name(), err)
			}

			parts = append(parts,
				ai.NewToolResponsePart(&ai.ToolResponse{
					Name:   req.Name,
					Ref:    req.Ref,
					Output: output,
				}))

		}

		if len(parts) > 0 {
			resp, err := genkit.Generate(ctx, g,
				ai.WithMessages(append(response.History(), ai.NewMessage(ai.RoleUser, nil, parts...))...),
				ai.WithTools(toolRefs...),
			)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("\n", resp.Text())
			conversationHistory = append(conversationHistory, resp.Message)
		} else {
			fmt.Println("\n", response.Text())
		}

	}

}
