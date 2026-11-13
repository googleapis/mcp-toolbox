import { InMemoryRunner, LlmAgent, LogLevel } from '@google/adk';
import { ToolboxClient } from '@toolbox-sdk/adk';

process.env.GOOGLE_GENAI_API_KEY = process.env.GOOGLE_API_KEY || 'your-api-key'; // Replace it with your API key

const systemPrompt = `
You're a helpful hotel assistant. You handle hotel searching, booking and
cancellations. When the user searches for a hotel, mention it's name, id,
location and price tier. Always mention hotel ids while performing any
searches. This is very important for any operations. For any bookings or
cancellations, please provide the appropriate confirmation. Be sure to
update checkin or checkout dates if mentioned by the user.
Don't ask for confirmations from the user.
`;

// Pre-Processing
function enforeBusinessRules({tool, args}) {
  const name = tool.name;
  console.log(`POLICY CHECK: Intercepting '${name}'`);

  if (name === "update-hotel" && args.checkin_date && args.checkout_date) {
    try {
      const start = new Date(args.checkin_date);
      const end = new Date(args.checkout_date);
      const duration = (end - start) / (1000 * 60 * 60 * 24); // days

      if (duration > 14) {
        console.log("BLOCKED: Stay too long");
        return "Error: Maximum stay duration is 14 days.";
      }
    } catch (e) {
      // Ignore invalid dates
    }
  }
  return undefined;
}

// Post-Processing
function enrichResponse({tool, response}) {
  const name = tool.name;
  console.log(`ENRICHING RESPONSE: Intercepting '${name}'`);
  if (name === "book-hotel") {
    let content = response;
    if (response && typeof response === "object") {
      content = response.content;
    }
    if (typeof content === "string" && !content.includes("Error")) {
      const loyaltyBonus = 500;
      const enrichedContent = `Booking Confirmed!\n You earned ${loyaltyBonus} Loyalty Points with this stay.\n\nSystem Details: ${content}`;

      if (response && typeof response === "object") {
        return { ...response, content: enrichedContent };
      }
      return enrichedContent;
    }
  }
  return response;
}

async function runTurn(runner, userId, sessionId, prompt) {
  console.log(`\nUSER: '${prompt}'`);
  const content = { role: 'user', parts: [{ text: prompt }] };
  const stream = runner.runAsync({ userId, sessionId, newMessage: content });
  
  let fullText = "";
  for await (const chunk of stream) {
      if (chunk.content && chunk.content.parts) {
          fullText += chunk.content.parts.map(p => p.text || "").join("");
      }
  }
  
  console.log("-".repeat(50));
  console.log(`AI: ${fullText}`);
}

export async function main() {
  const userId = 'test_user';
  const client = new ToolboxClient('http://127.0.0.1:5000');
  const tools = await client.loadToolset("my-toolset");
  
  const rootAgent = new LlmAgent({
    name: 'hotel_agent',
    model: 'gemini-3-flash-preview',
    description: 'Agent for hotel bookings and administration.',
    instruction: systemPrompt,
    tools: tools,
    // Add any pre- and post- processing callbacks
    beforeToolCallback: enforeBusinessRules,
    afterToolCallback: enrichResponse
  });

  const appName = rootAgent.name;
  const runner = new InMemoryRunner({ agent: rootAgent, appName, logLevel: LogLevel.ERROR });
  const session = await runner.sessionService.createSession({ appName, userId });

  // Turn 1: Booking
  await runTurn(runner, userId, session.id, "Book hotel with id 3.");

  // Turn 2: Policy Violation
  await runTurn(runner, userId, session.id, "Update my hotel with id 3 with checkin date 2025-01-18 and checkout date 2025-02-10");
}

main();
