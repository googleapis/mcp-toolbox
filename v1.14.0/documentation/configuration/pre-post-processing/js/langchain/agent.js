import { ToolboxClient } from "@toolbox-sdk/core";
import { ChatGoogleGenerativeAI } from "@langchain/google-genai";
import { createAgent, createMiddleware, ToolMessage } from "langchain";
import { tool } from "@langchain/core/tools";
import { fileURLToPath } from "url";
import process from "process";

const systemPrompt = `
You're a helpful hotel assistant. You handle hotel searching, booking and
cancellations. When the user searches for a hotel, mention it's name, id,
location and price tier. Always mention hotel ids while performing any
searches. This is very important for any operations. For any bookings or
cancellations, please provide the appropriate confirmation. Be sure to
update checkin or checkout dates if mentioned by the user.
Don't ask for confirmations from the user.
`;

const GOOGLE_API_KEY = process.env.GOOGLE_API_KEY || 'your-api-key'; // Replace it with your API key

const businessRulesMiddleware = createMiddleware({
  name: "BusinessRules",
  wrapToolCall: async (request, handler) => {
    const toolName = request.toolCall.name;
    const toolArgs = request.toolCall.args;
    console.log(`POLICY CHECK: Intercepting '${toolName}' running with args ${JSON.stringify(toolArgs)}`);
    if (toolName === "update-hotel" && toolArgs.checkin_date && toolArgs.checkout_date) {
      try {
        const start = new Date(toolArgs.checkin_date);
        const end = new Date(toolArgs.checkout_date);
        const duration = (end - start) / (1000 * 60 * 60 * 24); // days

        if (duration > 14) {
          console.log("BLOCKED: Stay too long");
          return ToolMessage({content:'Error: Maximum stay duration is 14 days.', status:"error"})
        }
      } catch (e) {
        // Ignore invalid dates
      }
    }
    return handler(request);
  }
});

const enrichmentMiddleware = createMiddleware({
  name: "Enrichment",
  wrapToolCall: async (request, handler) => {
    const result = await handler(request);
    const toolName = request.toolCall.name;
    
    let content = result;
    if (typeof result === 'object' && result !== null && result.content) {
        content = result.content;
    }
    if (toolName === "book-hotel" && typeof content === 'string' && !content.includes("Error")) {
        const loyaltyBonus = 500;
        const enrichedContent = `Booking Confirmed!\n You earned ${loyaltyBonus} Loyalty Points with this stay.\n\nSystem Details: ${content}`;
        if (typeof result === 'object' && result !== null) {
            result.content = enrichedContent;
            return result;
        }
        return enrichedContent;
    }
    return result;
  }
});

const queries = [
  "Book hotel with id 3.",
  "Update my hotel with id 3 with checkin date 2025-01-18 and checkout date 2025-02-10"
];

async function main() {
  const client = new ToolboxClient("http://127.0.0.1:5000");
  const rawTools = await client.loadToolset("my-toolset");
  const tools = rawTools
    .map(t => tool(t, {
        name: t.getName(),
        description: t.getDescription(),
        schema: t.getParamSchema()
  }));

  const model = new ChatGoogleGenerativeAI({
    model: "gemini-3-flash-preview",
  });

  const agent = createAgent({ 
    model: model, 
    tools: tools, 
    systemPrompt: systemPrompt, 
    middleware: [businessRulesMiddleware, enrichmentMiddleware] 
  });

  for (const query of queries) {
    console.log(`\nUSER: '${query}'`);
    const result = await agent.invoke({
      messages: [
        { role: "user", content: query},
      ],
    });
    console.log("-".repeat(50));
    console.log(`AI: ${result.messages[result.messages.length-1].content}`);
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main();
}

export { main };
