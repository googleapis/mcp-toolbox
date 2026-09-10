import asyncio
from datetime import datetime

from langchain.agents import create_agent
from langchain.agents.middleware import wrap_tool_call
from langchain_core.messages import ToolMessage
from langchain_google_genai import ChatGoogleGenerativeAI
from toolbox_langchain import ToolboxClient

system_prompt = """
  You're a helpful hotel assistant. You handle hotel searching, booking and
  cancellations. When the user searches for a hotel, mention it's name, id,
  location and price tier. Always mention hotel ids while performing any
  searches. This is very important for any operations. For any bookings or
  cancellations, please provide the appropriate confirmation. Be sure to
  update checkin or checkout dates if mentioned by the user.
  Don't ask for confirmations from the user.
"""


# Pre processing
@wrap_tool_call
async def enforce_business_rules(request, handler):
    """
    Business Logic Validation:
    Enforces max stay duration (e.g., max 14 days).
    """
    tool_call = request.tool_call
    name = tool_call["name"]
    args = tool_call["args"]

    print(f"POLICY CHECK: Intercepting '{name}'")

    if name == "update-hotel":
        if "checkin_date" in args and "checkout_date" in args:
            try:
                start = datetime.fromisoformat(args["checkin_date"])
                end = datetime.fromisoformat(args["checkout_date"])
                duration = (end - start).days

                if duration > 14:
                    print("BLOCKED: Stay too long")
                    return ToolMessage(
                        content="Error: Maximum stay duration is 14 days.",
                        tool_call_id=tool_call["id"],
                    )
            except ValueError:
                pass  # Ignore invalid date formats

    # PRE: Code here runs BEFORE the tool execution
    
    # EXEC: Execute the tool (or next middleware)
    result = await handler(request)

    # POST: Code here runs AFTER the tool execution
    return result


# Post processing
@wrap_tool_call
async def enrich_response(request, handler):
    """
    Post-Processing & Enrichment:
    Adds loyalty points information to successful bookings.
    Standardizes output format.
    """
    # PRE: Code here runs BEFORE the tool execution
    
    # EXEC: Execute the tool (or next middleware)
    result = await handler(request)

    # POST: Code here runs AFTER the tool execution
    if isinstance(result, ToolMessage):
        content = str(result.content)
        tool_name = request.tool_call["name"]

        if tool_name == "book-hotel" and "Error" not in content:
            loyalty_bonus = 500
            result.content = f"Booking Confirmed!\n You earned {loyalty_bonus} Loyalty Points with this stay.\n\nSystem Details: {content}"

    return result


async def main():
    async with ToolboxClient("http://127.0.0.1:5000") as client:
        tools = await client.aload_toolset("my-toolset")
        model = ChatGoogleGenerativeAI(model="gemini-3-flash-preview")
        agent = create_agent(
            system_prompt=system_prompt,
            model=model,
            tools=tools,
            # add any pre and post processing methods
            middleware=[enforce_business_rules, enrich_response],
        )
        # Test post-processing
        user_input = "Book hotel with id 3."
        print(f"\n[INPUT] User: {user_input}")
        response = await agent.ainvoke(
            {"messages": [{"role": "user", "content": user_input}]}
        )

        print("-" * 50)
        last_ai_msg = response["messages"][-1].content
        print(f"[OUTPUT] AI: {last_ai_msg}")

        # Test Pre-processing
        print("-" * 50)
        user_input = "Update my hotel with id 3 with checkin date 2025-01-18 and checkout date 2025-02-20."
        print(f"\n[INPUT] User: {user_input}")
        response = await agent.ainvoke(
            {"messages": [{"role": "user", "content": user_input}]}
        )
        last_ai_msg = response["messages"][-1].content
        print(f"[OUTPUT] AI: {last_ai_msg}")


if __name__ == "__main__":
    asyncio.run(main())
