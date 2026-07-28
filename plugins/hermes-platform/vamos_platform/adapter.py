"""Hermes platform adapter for authenticated Vamos shared threads.

The adapter exposes a small HTTP ingress for Vamos.  It intentionally builds
normal Hermes MessageEvents and delegates them to BasePlatformAdapter: Hermes
therefore owns session, background-task, and live-process state.  The adapter
never opens Hermes operational storage.

Configure under ``platforms.vamos.extra``.  ``host`` defaults to loopback;
set an explicit reverse-proxy/TLS endpoint before exposing it remotely.
"""

import asyncio
import hmac
import json
import logging
import uuid
from typing import Any, Optional

from aiohttp import web
from gateway.config import Platform, PlatformConfig
from gateway.platforms.base import BasePlatformAdapter, MessageEvent, MessageType, SendResult
from gateway.session import SessionSource

logger = logging.getLogger(__name__)

_LOOPBACK_HOST = "127.0.0.1"
_DEFAULT_PORT = 8765


class VamosAdapter(BasePlatformAdapter):
    """Receives Vamos thread events and sends Hermes output back to Vamos."""

    def __init__(self, config: PlatformConfig):
        super().__init__(config, Platform("vamos"))
        extra = config.extra or {}
        self._host = str(extra.get("host") or _LOOPBACK_HOST)
        self._port = int(extra.get("port") or _DEFAULT_PORT)
        self._token = str(extra.get("token") or "")
        self._vamos_url = str(extra.get("vamos_url") or "").rstrip("/")
        self._callback_token = str(extra.get("callback_token") or self._token)
        self._runner: Optional[web.AppRunner] = None
        self._client = None
        self._thread_plans: dict[str, str] = {}

    async def connect(self, *, is_reconnect: bool = False) -> bool:
        if not self._token or not self._vamos_url:
            logger.error("[vamos] token and vamos_url are required")
            return False
        app = web.Application()
        app.router.add_post("/vamos/prompts", self._prompt)
        app.router.add_post("/vamos/threads/{thread_id}/pi/{session_id}/complete", self._completion)
        app.router.add_get("/health", self._health)
        self._runner = web.AppRunner(app)
        await self._runner.setup()
        site = web.TCPSite(self._runner, self._host, self._port)
        await site.start()
        self._mark_connected()
        logger.info("[vamos] ingress listening on %s:%s", self._host, self._port)
        return True

    async def disconnect(self) -> None:
        if self._runner:
            await self._runner.cleanup()
            self._runner = None
        self._mark_disconnected()

    async def _health(self, request: web.Request) -> web.Response:
        return web.json_response({"status": "ok"})

    def _authorized(self, request: web.Request) -> bool:
        supplied = request.headers.get("Authorization", "").removeprefix("Bearer ")
        return bool(self._token) and hmac.compare_digest(supplied.encode(), self._token.encode())

    async def _body(self, request: web.Request) -> dict[str, Any]:
        if not self._authorized(request):
            raise web.HTTPUnauthorized(text="invalid Vamos credential")
        try:
            body = await request.json()
        except (json.JSONDecodeError, ValueError):
            raise web.HTTPBadRequest(text="invalid JSON")
        if not isinstance(body, dict):
            raise web.HTTPBadRequest(text="object payload required")
        return body

    async def _prompt(self, request: web.Request) -> web.Response:
        body = await self._body(request)
        thread_id = str(body.get("thread_id") or "").strip()
        owner = str(body.get("owner_email") or "").strip()
        prompt = str(body.get("prompt") or "").strip()
        if not thread_id or not owner or not prompt:
            raise web.HTTPBadRequest(text="thread_id, owner_email, and prompt are required")
        plan_dir = str(body.get("plan_dir") or "").strip()
        if plan_dir:
            self._thread_plans[thread_id] = plan_dir
        event = MessageEvent(
            text=prompt,
            message_type=MessageType.TEXT,
            source=SessionSource(
                platform=Platform("vamos"), chat_id=thread_id, chat_name="Vamos",
                chat_type="thread", thread_id=thread_id, user_id=owner, user_name=owner,
            ),
            raw_message=body,
            message_id=uuid.uuid4().hex,
        )
        await self.handle_message(event)
        return web.json_response({"status": "accepted"}, status=202)

    async def _completion(self, request: web.Request) -> web.Response:
        body = await self._body(request)
        thread_id = request.match_info["thread_id"]
        session_id = request.match_info["session_id"]
        result = json.dumps(body, sort_keys=True)
        event = MessageEvent(
            text=f"Pi session {session_id} completed. Result:\n{result}",
            message_type=MessageType.TEXT,
            source=SessionSource(platform=Platform("vamos"), chat_id=thread_id,
                chat_name="Vamos", chat_type="thread", thread_id=thread_id,
                user_id="vamos", user_name="Vamos"),
            raw_message={"pi_session_id": session_id, "result": body},
            message_id=uuid.uuid4().hex,
            internal=True,
        )
        await self.handle_message(event)
        return web.json_response({"status": "accepted"}, status=202)

    async def send(self, chat_id: str, content: str, reply_to: Optional[str] = None,
                   metadata: Optional[dict[str, Any]] = None) -> SendResult:
        """Deliver final Markdown through Vamos's authenticated callback."""
        import aiohttp
        if self._client is None:
            self._client = aiohttp.ClientSession()
        event = {
            "id": uuid.uuid4().hex,
            "type": "final",
            "content": content,
            "plan_dir": self._thread_plans.get(chat_id, ""),
        }
        url = f"{self._vamos_url}/agent-chat/api/hermes/threads/{chat_id}/events"
        try:
            async with self._client.post(url, json=event, headers={"Authorization": f"Bearer {self._callback_token}"}) as response:
                if response.status // 100 != 2:
                    return SendResult(success=False, error=f"Vamos callback: {response.status}")
        except Exception as exc:
            return SendResult(success=False, error=str(exc))
        return SendResult(success=True, message_id=event["id"])

    async def get_chat_info(self, chat_id: str) -> dict[str, Any]:
        return {"name": "Vamos", "type": "thread"}


def check_requirements() -> bool:
    return True


def validate_config(config: PlatformConfig) -> bool:
    extra = config.extra or {}
    return bool(extra.get("token") and extra.get("vamos_url"))


def register(ctx) -> None:
    ctx.register_platform(
        name="vamos", label="Vamos", adapter_factory=lambda cfg: VamosAdapter(cfg),
        check_fn=check_requirements, validate_config=validate_config,
        emoji="🧵", pii_safe=False,
    )
