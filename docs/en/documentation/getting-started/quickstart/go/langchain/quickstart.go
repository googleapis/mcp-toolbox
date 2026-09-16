package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

// ConvertToLangchainTool converts a generic core.ToolboxTool into a LangChainGo llms.Tool.
func ConvertToLangchainTool(toolboxTool *core.ToolboxTool) llms.Tool {

	// Fetch the tool's input schema
	inputschema, err := toolboxTool.InputSchema()
	if err != nil {
		return llms.Tool{}
	}

	var paramsSchema map[string]any
	_ = json.Unmarshal(inputschema, &paramsSchema)

	// Convert into LangChain's llms.Tool
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        toolboxTool.Name(),
			Description: toolboxTool.Description(),
			Parameters:  paramsSchema,
		},
	}
}

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
	"Oh wait, this is too expensive. Please cancel it.",
	"Please book the Hyatt Regency instead.",
	"My check in dates would be from April 10, 2024 to April 19, 2024.",
}

func main() {
	genaiKey := os.Getenv("GOOGLE_API_KEY")
	toolboxURL := "http://localhost:5000"
	ctx := context.Background()

	// Initialize the Google AI client (LLM).
	llm, err := googleai.New(ctx, googleai.WithAPIKey(genaiKey), googleai.WithDefaultModel("gemini-3.6-flash"))
	if err != nil {
		log.Fatalf("Failed to create Google AI client: %v", err)
	}

	// Initialize the MCP Toolbox client.
	toolboxClient, err := core.NewToolboxClient(toolboxURL)
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Load the tool using the MCP Toolbox SDK.
	tools, err := toolboxClient.LoadToolset("my-toolset", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v\nMake sure your Toolbox server is running and the tool is configured.", err)
	}

	toolsMap := make(map[string]*core.ToolboxTool, len(tools))

	langchainTools := make([]llms.Tool, len(tools))
	// Convert the loaded ToolboxTools into the format LangChainGo requires.
	for i, tool := range tools {
		langchainTools[i] = ConvertToLangchainTool(tool)
		toolsMap[tool.Name()] = tool
	}

	// Start the conversation history.
	messageHistory := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt),
	}

	for _, query := range queries {
		messageHistory = append(messageHistory, llms.TextParts(llms.ChatMessageTypeHuman, query))

		// Make the first call to the LLM, making it aware of the tool.
		resp, err := llm.GenerateContent(ctx, messageHistory, llms.WithTools(langchainTools))
		if err != nil {
			log.Fatalf("LLM call failed: %v", err)
		}
		respChoice := resp.Choices[0]

		assistantResponse := llms.TextParts(llms.ChatMessageTypeAI, respChoice.Content)
		for _, tc := range respChoice.ToolCalls {
			assistantResponse.Parts = append(assistantResponse.Parts, tc)
		}
		messageHistory = append(messageHistory, assistantResponse)

		// Process each tool call requested by the model.
		for _, tc := range respChoice.ToolCalls {
			toolName := tc.FunctionCall.Name
			tool := toolsMap[toolName]
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.FunctionCall.Arguments), &args); err != nil {
				log.Fatalf("Failed to unmarshal arguments for tool '%s': %v", toolName, err)
			}
			toolResult, err := tool.Invoke(ctx, args)
			if err != nil {
				log.Fatalf("Failed to execute tool '%s': %v", toolName, err)
			}
			if toolResult == "" || toolResult == nil {
				toolResult = "Operation completed successfully with no specific return value."
			}

			// Create the tool call response message and add it to the history.
			toolResponse := llms.MessageContent{
				Role: llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{
					llms.ToolCallResponse{
						Name:    toolName,
						Content: fmt.Sprintf("%v", toolResult),
					},
				},
			}
			messageHistory = append(messageHistory, toolResponse)
		}
		finalResp, err := llm.GenerateContent(ctx, messageHistory)
		if err != nil {
			log.Fatalf("Final LLM call failed after tool execution: %v", err)
		}

		// Add the final textual response from the LLM to the history
		messageHistory = append(messageHistory, llms.TextParts(llms.ChatMessageTypeAI, finalResp.Choices[0].Content))

		fmt.Println(finalResp.Choices[0].Content)

	}

}
