import os

import httpx
from aiogram import Bot, Dispatcher, F
from aiogram.filters import Command
from aiogram.types import Message

BRAIN_URL = os.getenv("BRAIN_URL", "http://hermes-brain:8088")
TOKEN = os.getenv("TELEGRAM_BOT_TOKEN")
ALLOWED = {int(x) for x in os.getenv("TELEGRAM_ALLOWED_USER_IDS", "").replace(" ", "").split(",") if x}


def allowed(message: Message) -> bool:
    return not ALLOWED or (message.from_user and message.from_user.id in ALLOWED)


async def main() -> None:
    if not TOKEN:
        raise RuntimeError("TELEGRAM_BOT_TOKEN is required")
    bot = Bot(TOKEN)
    dp = Dispatcher()

    @dp.message(Command("start"))
    async def start(message: Message):
        if not allowed(message):
            return await message.answer("Access denied")
        await message.answer("Hermes/OpenCode team ready. Use /task <text>, /approve <session_id> <agent> <task>, /memory <session_id>.")

    @dp.message(Command("task"))
    async def task(message: Message):
        if not allowed(message):
            return await message.answer("Access denied")
        text = message.text.partition(" ")[2].strip()
        if not text:
            return await message.answer("Usage: /task implement health endpoint")
        await message.answer("Task accepted. Running agents...")
        async with httpx.AsyncClient(timeout=900) as client:
            r = await client.post(f"{BRAIN_URL}/workflow/run", json={"task": text})
            r.raise_for_status()
            data = r.json()
        await message.answer(f"session_id: {data['session_id']}\n\n{data['summary'][:3500]}")

    @dp.message(Command("approve"))
    async def approve(message: Message):
        if not allowed(message):
            return await message.answer("Access denied")
        _, _, rest = message.text.partition(" ")
        parts = rest.split(" ", 2)
        if len(parts) < 3:
            return await message.answer("Usage: /approve <session_id> <agent> <task>")
        session_id, agent, task_text = parts
        await message.answer("Approval accepted. Running code engine...")
        async with httpx.AsyncClient(timeout=1900) as client:
            r = await client.post(f"{BRAIN_URL}/workflow/approve", json={"session_id": session_id, "agent": agent, "task": task_text})
            r.raise_for_status()
            data = r.json()
        await message.answer((data.get("stdout") or data.get("stderr") or str(data))[:3500])

    @dp.message(Command("memory"))
    async def memory(message: Message):
        if not allowed(message):
            return await message.answer("Access denied")
        session_id = message.text.partition(" ")[2].strip()
        async with httpx.AsyncClient(timeout=60) as client:
            r = await client.get(f"{BRAIN_URL}/memory/{session_id}")
            r.raise_for_status()
            data = r.json()
        text = "\n".join(f"{e['role']}: {e['content'][:300]}" for e in data["events"][-10:])
        await message.answer(text[:3500] or "No memory")

    @dp.message(F.text)
    async def fallback(message: Message):
        if allowed(message):
            await message.answer("Use /task <text>")

    await dp.start_polling(bot)


if __name__ == "__main__":
    import asyncio
    asyncio.run(main())
