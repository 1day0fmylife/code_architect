from datetime import datetime
from sqlalchemy import DateTime, Integer, String, Text, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from app.core.settings import settings


class Base(DeclarativeBase):
    pass


class MemoryEvent(Base):
    __tablename__ = "memory_events"
    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    session_id: Mapped[str] = mapped_column(String(128), index=True)
    role: Mapped[str] = mapped_column(String(64), index=True)
    content: Mapped[str] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, index=True)


engine = create_async_engine(settings.database_url, pool_pre_ping=True)
SessionLocal = async_sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)


async def init_db() -> None:
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)


async def remember(session_id: str, role: str, content: str) -> None:
    async with SessionLocal() as session:
        session.add(MemoryEvent(session_id=session_id, role=role, content=content[:20000]))
        await session.commit()


async def recall(session_id: str, limit: int = 20) -> list[MemoryEvent]:
    async with SessionLocal() as session:
        rows = await session.execute(
            select(MemoryEvent)
            .where(MemoryEvent.session_id == session_id)
            .order_by(MemoryEvent.id.desc())
            .limit(limit)
        )
        return list(reversed(rows.scalars().all()))
