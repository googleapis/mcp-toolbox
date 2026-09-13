import com.google.cloud.mcp.McpToolboxClient;
import com.google.cloud.mcp.tool.ToolDefinition;
import com.google.cloud.mcp.tool.ToolResult;
import java.util.Map;

public class Quickstart {
  public static void main(String[] args) {
    // Connect to the local MCP Toolbox server
    McpToolboxClient client =
        McpToolboxClient.builder().baseUrl("http://127.0.0.1:5000").build();

    // 1. Load tools from the toolset
    Map<String, ToolDefinition> tools = client.loadToolset("my-toolset").join();
    System.out.println("Available tools: " + tools.keySet());

    // 2. Search for hotels by name
    System.out.println("\n--- Searching for hotels named 'Hilton' ---");
    ToolResult searchResult =
        client.invokeTool("search-hotels-by-name", Map.of("name", "Hilton")).join();
    System.out.println("Result: " + searchResult.content().get(0).text());

    // 3. Book a hotel by ID
    System.out.println("\n--- Booking hotel with ID 1 ---");
    ToolResult bookResult = client.invokeTool("book-hotel", Map.of("hotel_id", "1")).join();
    System.out.println("Booking status: " + (bookResult.isError() ? "Failed" : "Success"));
  }
}
