import asyncio

from langgraph.prebuilt import create_react_agent

# TODO(developer): replace this with another import if needed
from langchain_google_genai import ChatGoogleGenerativeAI
# from langchain_anthropic import ChatAnthropic

from langgraph.checkpoint.memory import MemorySaver
from toolbox_langchain import ToolboxClient

prompt = """
  You're a helpful hotel assistant. You handle hotel searching, booking and
  cancellations. When the user searches for a hotel, mention it's name, id,
  location and price tier. Always mention hotel ids while performing any
  searches. This is very important for any operations. For any bookings or
  cancellations, please provide the appropriate confirmation. Be sure to
  update checkin or checkout dates if mentioned by the user.
  Don't ask for confirmations from the user.
"""

queries = [
    "Find hotels in Basel with Basel in its name.",
    "Can you book the Hilton Basel for me?",
    "Oh wait, this is too expensive. Please cancel it and book the Hyatt Regency instead.",
    "My check in dates would be from April 10, 2024 to April 19, 2024.",
]

async def main():
    # TODO(developer): replace this with another model if needed
    model = ChatGoogleGenerativeAI(model="gemini-3-flash-preview")
    # model = ChatAnthropic(model="claude-3-5-sonnet-20240620")

    # Load the tools from the Toolbox server
    async with ToolboxClient("http://127.0.0.1:5000") as client:
        tools = await client.aload_toolset()

        agent = create_react_agent(model, tools, checkpointer=MemorySaver())

        config = {"configurable": {"thread_id": "thread-1"}}
        for query in queries:
            inputs = {"messages": [("user", prompt + query)]}
            print(f"\n[INPUT] User: {query}")
            response = agent.invoke(inputs, stream_mode="values", config=config)
            print(f"[OUTPUT] AI: {response['messages'][-1].content}")

asyncio.run(main())
