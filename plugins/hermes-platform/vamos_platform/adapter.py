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

from . import session_ingress_v1

logger = logging.getLogger(__name__)

_LOOPBACK_HOST = "127.0.0.1"
_DEFAULT_PORT = 8765


class VamosAdapter(BasePlatformAdapter):
    """Receives Vamos thread events and sends Hermes output back to Vamos."""

    gateway_runner = None

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
        app.router.add_post("/vamos/manager-wake", self._manager_wake)
        app.router.add_post(
            "/vamos/session-ingress/v1/capabilities", self._session_ingress_capabilities,
        )
        app.router.add_post(
            "/vamos/session-ingress/v1/enqueue", self._session_ingress_enqueue,
        )
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

    def _v1_response(self, value) -> web.Response:
        payload = session_ingress_v1.canonical_json(value)
        code = value["code"] if isinstance(value, dict) else value.code
        return web.Response(
            body=payload,
            status=session_ingress_v1.http_status(code),
            content_type="application/json",
        )

    async def _v1_request(self, request: web.Request, expected_op: str):
        if not self._authorized(request):
            return None, self._v1_response(session_ingress_v1.rejection("unauthorized"))
        if request.content_type != "application/json":
            return None, self._v1_response(session_ingress_v1.rejection("malformed"))
        identity_headers = {
            "hermes-session-id", "pi-session-id", "message-id",
            "x-hermes-session-id", "x-pi-session-id", "x-message-id",
        }
        if request.query or request.cookies or any(
            key.lower() in identity_headers for key in request.headers
        ):
            return None, self._v1_response(session_ingress_v1.rejection("malformed"))
        if request.content_length is not None and request.content_length > session_ingress_v1.MAX_BODY_BYTES:
            return None, self._v1_response(session_ingress_v1.rejection("malformed"))
        body = bytearray()
        async for chunk in request.content.iter_chunked(64 * 1024):
            body.extend(chunk)
            if len(body) > session_ingress_v1.MAX_BODY_BYTES:
                return None, self._v1_response(session_ingress_v1.rejection("malformed"))
        try:
            return session_ingress_v1.parse_request(bytes(body), expected_op), None
        except session_ingress_v1.ProtocolError as exc:
            return None, self._v1_response(session_ingress_v1.rejection(exc.code))

    async def _session_ingress_capabilities(self, request: web.Request) -> web.Response:
        _, error = await self._v1_request(request, "capabilities")
        if error is not None:
            return error
        if not session_ingress_v1.runner_supports_v1(self.gateway_runner):
            return self._v1_response(session_ingress_v1.rejection("surface_unsupported"))
        return self._v1_response(session_ingress_v1.capability_response())

    async def _session_ingress_enqueue(self, request: web.Request) -> web.Response:
        parsed, error = await self._v1_request(request, "enqueue")
        if error is not None:
            return error
        runner = self.gateway_runner
        if not session_ingress_v1.runner_supports_v1(runner):
            return self._v1_response(session_ingress_v1.rejection("surface_unsupported"))
        result = await runner.enqueue_internal_session_turn(
            parsed.hermes_session_id,
            parsed.pi_session_id,
            parsed.message_id,
            session_ingress_v1.manager_turn(parsed),
        )
        try:
            response = session_ingress_v1.result_response(result.code)
        except session_ingress_v1.ProtocolError:
            response = session_ingress_v1.rejection("temporarily_unavailable")
        return self._v1_response(response)

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

    @staticmethod
    def _manager_wake_body(body: dict[str, Any]) -> dict[str, Any]:
        if type(body.get("version")) is not int or body["version"] != 1:
            raise web.HTTPBadRequest(text="version must be integer 1")
        for field in ("manager_thread_id", "pi_session_id", "message_id"):
            value = body.get(field)
            if not isinstance(value, str) or not value:
                raise web.HTTPBadRequest(text=f"{field} must be a non-empty string")
        if not isinstance(body.get("message"), str):
            raise web.HTTPBadRequest(text="message must be a string")
        return body

    async def _manager_wake(self, request: web.Request) -> web.Response:
        body = self._manager_wake_body(await self._body(request))
        thread_id = body["manager_thread_id"]
        event = MessageEvent(
            text=body["message"],
            message_type=MessageType.TEXT,
            source=SessionSource(
                platform=Platform("vamos"), chat_id=thread_id, chat_name="Vamos",
                chat_type="thread", thread_id=thread_id, user_id="vamos", user_name="Vamos",
            ),
            raw_message={
                "pi_session_id": body["pi_session_id"],
                "message_id": body["message_id"],
            },
            message_id=body["message_id"],
            internal=True,
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
