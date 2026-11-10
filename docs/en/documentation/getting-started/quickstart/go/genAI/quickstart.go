package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	"google.golang.org/genai"
)

// ConvertToGenaiTool translates a ToolboxTool into the genai.FunctionDeclaration format.
func ConvertToGenaiTool(toolboxTool *core.ToolboxTool) *genai.Tool {

	inputschema, err := toolboxTool.InputSchema()
	if err != nil {
		return &genai.Tool{}
	}

	var paramsSchema *genai.Schema
	_ = json.Unmarshal(inputschema, &paramsSchema)
	// First, create the function declaration.
	funcDeclaration := &genai.FunctionDeclaration{
		Name:        toolboxTool.Name(),
		Description: toolboxTool.Description(),
		Parameters:  paramsSchema,
	}

	// Then, wrap the function declaration in a genai.Tool struct.
	return &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{funcDeclaration},
	}
}

func printResponse(resp *genai.GenerateContentResponse) {
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				fmt.Println(part.Text)
			}
		}
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
	// Setup
	ctx := context.Background()
	apiKey := os.Getenv("GOOGLE_API_KEY")
	toolboxURL := "http://localhost:5000"

	// Initialize the Google GenAI client using the explicit ClientConfig.
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create Google GenAI client: %v", err)
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

	genAITools := make([]*genai.Tool, len(tools))
	toolsMap := make(map[string]*core.ToolboxTool, len(tools))

	for i, tool := range tools {
		genAITools[i] = ConvertToGenaiTool(tool)
		toolsMap[tool.Name()] = tool
	}

	// Set up the generative model with the available tool.
	modelName := "gemini-3.6-flash"

	// Create the initial content prompt for the model.
	messageHistory := []*genai.Content{
		genai.NewContentFromText(systemPrompt, genai.RoleUser),
	}
	config := &genai.GenerateContentConfig{
		Tools: genAITools,
		ToolConfig: &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAny,
			},
		},
	}

	for _, query := range queries {

		messageHistory = append(messageHistory, genai.NewContentFromText(query, genai.RoleUser))

		genContentResp, err := client.Models.GenerateContent(ctx, modelName, messageHistory, config)
		if err != nil {
			log.Fatalf("LLM call failed for query '%s': %v", query, err)
		}

		if len(genContentResp.Candidates) > 0 && genContentResp.Candidates[0].Content != nil {
			messageHistory = append(messageHistory, genContentResp.Candidates[0].Content)
		}

		functionCalls := genContentResp.FunctionCalls()

		toolResponseParts := []*genai.Part{}

		for _, fc := range functionCalls {

			toolToInvoke, found := toolsMap[fc.Name]
			if !found {
				log.Fatalf("Tool '%s' not found in loaded tools map. Check toolset configuration.", fc.Name)
			}

			toolResult, invokeErr := toolToInvoke.Invoke(ctx, fc.Args)
			if invokeErr != nil {
				log.Fatalf("Failed to execute tool '%s': %v", fc.Name, invokeErr)
			}

			// Enhanced Tool Result Handling (retained to prevent nil issues)
			toolResultString := ""
			if toolResult != nil {
				jsonBytes, marshalErr := json.Marshal(toolResult)
				if marshalErr == nil {
					toolResultString = string(jsonBytes)
				} else {
					toolResultString = fmt.Sprintf("%v", toolResult)
				}
			}

			responseMap := map[string]any{"result": toolResultString}

			toolResponseParts = append(toolResponseParts, genai.NewPartFromFunctionResponse(fc.Name, responseMap))
		}
		// Add all accumulated tool responses for this turn to the message history.
		toolResponseContent := genai.NewContentFromParts(toolResponseParts, genai.RoleUser)
		messageHistory = append(messageHistory, toolResponseContent)

		finalResponse, err := client.Models.GenerateContent(ctx, modelName, messageHistory, &genai.GenerateContentConfig{})
		if err != nil {
			log.Fatalf("Error calling GenerateContent (with function result): %v", err)
		}

		printResponse(finalResponse)
		// Add the final textual response from the LLM to the history
		if len(finalResponse.Candidates) > 0 && finalResponse.Candidates[0].Content != nil {
			messageHistory = append(messageHistory, finalResponse.Candidates[0].Content)
		}
	}
}
