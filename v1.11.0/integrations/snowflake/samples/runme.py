import asyncio
from toolbox_core import ToolboxClient


async def main():
    # Replace with the actual URL where your Toolbox service is running
    async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
        tool = await toolbox.load_tool("execute_sql")
        result = await tool("SELECT 1")
        print(result)

        tool = await toolbox.load_tool("list_tables")
        result = await tool(table_names="DIM_DATE")
        print(result)


if __name__ == "__main__":
    asyncio.run(main())
