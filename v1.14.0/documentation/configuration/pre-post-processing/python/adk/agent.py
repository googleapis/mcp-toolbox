import asyncio
from datetime import datetime
from typing import Any, Dict, Optional
from copy import deepcopy

from google.adk import Agent
from google.adk.apps import App
from google.adk.runners import Runner
from google.adk.sessions.in_memory_session_service import InMemorySessionService
from google.adk.tools.tool_context import ToolContext
from google.genai import types
from toolbox_adk import CredentialStrategy, ToolboxToolset, ToolboxTool

SYSTEM_PROMPT = """
  You're a helpful hotel assistant. You handle hotel searching, booking and
  cancellations. When the user searches for a hotel, mention it's name, id,
  location and price tier. Always mention hotel ids while performing any
  searches. This is very important for any operations. For any bookings or
  cancellations, please provide the appropriate confirmation. Be sure to
  update checkin or checkout dates if mentioned by the user.
  Don't ask for confirmations from the user.
"""


# Pre processing
async def enfore_business_rules(
    tool: ToolboxTool, args: Dict[str, Any], tool_context: ToolContext
) -> Optional[Dict[str, Any]]:
    """
    Callback fired before a tool is executed.
    Enforces business logic: Max stay duration is 14 days.
    """
    tool_name = tool.name
    print(f"POLICY CHECK: Intercepting '{tool_name}'")

    if tool_name == "update-hotel" and "checkin_date" in args and "checkout_date" in args:
        start = datetime.fromisoformat(args["checkin_date"])
        end = datetime.fromisoformat(args["checkout_date"])
        duration = (end - start).days

        if duration > 14:
            print("BLOCKED: Stay too long")
            return {"result": "Error: Maximum stay duration is 14 days."}
    return None


# Post processing
async def enrich_response(
    tool: ToolboxTool,
    args: Dict[str, Any],
    tool_context: ToolContext,
    tool_response: Any,
) -> Optional[Any]:
    """
    Callback fired after a tool execution.
    Enriches response for successful bookings.
    """
    if isinstance(tool_response, dict):
        result = tool_response.get("result", "")
    elif isinstance(tool_response, str):
        result = tool_response
    else:
        return None

    tool_name = tool.name
    if isinstance(result, str) and "Error" not in result:
        if tool_name == "book-hotel":
            loyalty_bonus = 500
            enriched_result = f"Booking Confirmed!\n You earned {loyalty_bonus} Loyalty Points with this stay.\n\nSystem Details: {result}"

            if isinstance(tool_response, dict):
                modified_response = deepcopy(tool_response)
                modified_response["result"] = enriched_result
                return modified_response
            else:
                return enriched_result
    return None


async def run_chat_turn(
    runner: Runner, session_id: str, user_id: str, message_text: str
):
    """Executes a single chat turn and prints the interaction."""
    print(f"\nUSER: '{message_text}'")
    response_text = ""
    async for event in runner.run_async(
        user_id=user_id,
        session_id=session_id,
        new_message=types.Content(role="user", parts=[types.Part(text=message_text)]),
    ):
        if event.content and event.content.parts:
            for part in event.content.parts:
                if part.text:
                    response_text += part.text

    print(f"AI: {response_text}")


async def main():
    toolset = ToolboxToolset(
        server_url="http://127.0.0.1:5000",
        toolset_name="my-toolset",
        credentials=CredentialStrategy.toolbox_identity(),
    )
    tools = await toolset.get_tools()
    root_agent = Agent(
        name="root_agent",
        model="gemini-3-flash-preview",
        instruction=SYSTEM_PROMPT,
        tools=tools,
        # add any pre and post processing callbacks
        before_tool_callback=enfore_business_rules,
        after_tool_callback=enrich_response,
    )
    app = App(root_agent=root_agent, name="my_agent")
    runner = Runner(app=app, session_service=InMemorySessionService())
    session_id = "test-session"
    user_id = "test-user"
    await runner.session_service.create_session(
        app_name=app.name, user_id=user_id, session_id=session_id
    )

    # First turn: Successful booking
    await run_chat_turn(runner, session_id, user_id, "Book hotel with id 3.")
    print("-" * 50)
    # Second turn: Policy violation (stay > 14 days)
    await run_chat_turn(
        runner,
        session_id,
        user_id,
        "Update hotel with id 5 with checkin date 2025-01-18 and checkout date 2025-02-10",
    )
    await toolset.close()


if __name__ == "__main__":
    asyncio.run(main())
