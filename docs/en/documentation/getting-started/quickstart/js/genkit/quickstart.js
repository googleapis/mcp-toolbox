import { ToolboxClient } from "@toolbox-sdk/core";
import { genkit } from "genkit";
import { googleAI } from '@genkit-ai/google-genai'; 

const GOOGLE_API_KEY = process.env.GOOGLE_API_KEY || 'your-api-key'; // Replace it with your API key

const systemPrompt = `
You're a helpful hotel assistant. You handle hotel searching, booking, and
cancellations. When the user searches for a hotel, mention its name, id,
location and price tier. Always mention hotel ids while performing any
searches. This is very important for any operations. For any bookings or
cancellations, please provide the appropriate confirmation. Be sure to
update checkin or checkout dates if mentioned by the user.
Don't ask for confirmations from the user.
`;

const queries = [
  "Find hotels in Basel with Basel in its name.",
  "Can you book the Hilton Basel for me?",
  "Oh wait, this is too expensive. Please cancel it and book the Hyatt Regency instead.",
  "My check in dates would be from April 10, 2024 to April 19, 2024.",
];

export async function main() {
  const toolboxClient = new ToolboxClient("http://127.0.0.1:5000");

  const ai = genkit({
    plugins: [
      googleAI({
        apiKey: process.env.GEMINI_API_KEY || GOOGLE_API_KEY
      })
    ],
    model: googleAI.model('gemini-3-flash-preview'),
  });

  const toolboxTools = await toolboxClient.loadToolset("my-toolset");
  const toolMap = Object.fromEntries(
    toolboxTools.map((tool) => {
      const definedTool = ai.defineTool(
        {
          name: tool.getName(),
          description: tool.getDescription(),
          // @genkit-ai/core validates inputJsonSchema with Ajv (configured for JSON Schema draft-07).
          inputJsonSchema: tool.getParamSchema().toJSONSchema({ target: "draft-7" }),
        },
        tool
      );
      return [tool.getName(), definedTool];
    })
  );
  const tools = Object.values(toolMap);

  let conversationHistory = [{ role: "system", content: [{ text: systemPrompt }] }];

  for (const query of queries) {
    conversationHistory.push({ role: "user", content: [{ text: query }] });
    let response = await ai.generate({
      messages: conversationHistory,
      tools: tools,
    });
    conversationHistory.push(response.message);

    const toolRequests = response.toolRequests;
    if (toolRequests?.length > 0) {
      // Execute tools concurrently and collect their responses.
      const toolResponses = await Promise.all(
        toolRequests.map(async (call) => {
          try {
            const toolOutput = await toolMap[call.name].invoke(call.input);
            return { role: "tool", content: [{ toolResponse: { name: call.name, output: toolOutput } }] };
          } catch (e) {
            console.error(`Error executing tool ${call.name}:`, e);
            return { role: "tool", content: [{ toolResponse: { name: call.name, output: { error: e.message } } }] };
          }
        })
      );

      conversationHistory.push(...toolResponses);

      // Call the AI again with the tool results.
      response = await ai.generate({ messages: conversationHistory, tools });
      conversationHistory.push(response.message);
    }

    console.log(response.text);
  }
}

main();
