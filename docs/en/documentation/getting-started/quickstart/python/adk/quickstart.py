# [START quickstart]
import asyncio

from google.adk import Agent
from google.adk.apps import App
from google.adk.runners import InMemoryRunner
from google.adk.tools.toolbox_toolset import ToolboxToolset
from google.genai.types import Content, Part

prompt = """
You're a helpful hotel assistant. You handle hotel searching, booking and
cancellations. When the user searches for a hotel, mention it's name, id,
location and price tier. Always mention hotel ids while performing any
searches. This is very important for any operations. For any bookings or
cancellations, please provide the appropriate confirmation. Be sure to
update checkin or checkout dates if mentioned by the user.
Don't ask for confirmations from the user.
"""

# TODO(developer): update the TOOLBOX_URL to your toolbox endpoint
toolset = ToolboxToolset(
    server_url="http://127.0.0.1:5000",
)

root_agent = Agent(
    name='hotel_assistant',
    model='gemini-3-flash-preview',
    instruction=prompt,
    tools=[toolset],
)

app = App(root_agent=root_agent, name="my_agent")
# [END quickstart]

queries = [
    "Find hotels in Basel with Basel in its name.",
    "Can you book the Hilton Basel for me?",
    "Oh wait, this is too expensive. Please cancel it and book the Hyatt Regency instead.",
    "My check in dates would be from April 10, 2024 to April 19, 2024.",
]

async def main():
    runner = InMemoryRunner(app=app)
    session = await runner.session_service.create_session(
        app_name=app.name, user_id="test_user"
    )

    for query in queries:
        print(f"\nUser: {query}")
        user_message = Content(parts=[Part.from_text(text=query)])

        async for event in runner.run_async(user_id="test_user", session_id=session.id, new_message=user_message):
            if event.is_final_response() and event.content and event.content.parts:
                print(f"Agent: {event.content.parts[0].text}")

if __name__ == "__main__":
    asyncio.run(main())
