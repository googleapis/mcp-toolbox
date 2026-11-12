import { GoogleGenAI } from "@google/genai";
import { ToolboxClient } from "@toolbox-sdk/core";


const TOOLBOX_URL = "http://127.0.0.1:5000"; // Update if needed
const GOOGLE_API_KEY = process.env.GOOGLE_API_KEY || 'your-api-key'; // Replace it with your API key

const prompt = `
You're a helpful hotel assistant. You handle hotel searching, booking, and
cancellations. When the user searches for a hotel, you MUST use the available tools to find information. Mention its name, id,
location and price tier. Always mention hotel id while performing any
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

function mapZodTypeToOpenAPIType(zodTypeName) {

    const typeMap = {
        'ZodString': 'string',
        'ZodNumber': 'number',
        'ZodBoolean': 'boolean',
        'ZodArray': 'array',
        'ZodObject': 'object',
    };
    return typeMap[zodTypeName] || 'string';
}

export async function main() {
   
    const toolboxClient = new ToolboxClient(TOOLBOX_URL); 
    const toolboxTools = await toolboxClient.loadToolset("my-toolset");
    
    const geminiTools = [{
        functionDeclarations: toolboxTools.map(tool => {
            
            const schema = tool.getParamSchema();
            const properties = {};
            const required = [];

         
            for (const [key, param] of Object.entries(schema.shape)) {
                properties[key] = {
                        type: mapZodTypeToOpenAPIType(param.constructor.name),
                        description: param.description || '',
                    };
                required.push(key)
                }
            
            return {
                name: tool.getName(),
                description: tool.getDescription(),
                parameters: { type: 'object', properties, required },
            };
        })
    }];


    const genAI = new GoogleGenAI({ apiKey: GOOGLE_API_KEY });
    
    const chat = genAI.chats.create({
        model: "gemini-2.5-flash",
        config: {
            systemInstruction: prompt,
            tools: geminiTools,
        }
    });

    for (const query of queries) {
        
        let currentResult = await chat.sendMessage({ message: query });
        
        let finalResponseGiven = false
        while (!finalResponseGiven) {
            
            const response = currentResult;
            const functionCalls = response.functionCalls || [];

            if (functionCalls.length === 0) {
                console.log(response.text)
                finalResponseGiven = true;
            } else {
                const toolResponses = [];
                for (const call of functionCalls) {
                    const toolName = call.name
                    const toolToExecute = toolboxTools.find(t => t.getName() === toolName);
                    
                    if (toolToExecute) {
                        try {
                            const functionResult = await toolToExecute(call.args);
                            toolResponses.push({
                                functionResponse: { name: call.name, response: { result: functionResult } }
                            });
                        } catch (e) {
                            console.error(`Error executing tool '${toolName}':`, e);
                            toolResponses.push({
                                functionResponse: { name: call.name, response: { error: e.message } }
                            });
                        }
                    }
                }
                
                currentResult = await chat.sendMessage({ message: toolResponses });
            }
        }
        
    }
}

main();
