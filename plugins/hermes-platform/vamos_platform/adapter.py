"""Hermes platform adapter for authenticated Vamos shared threads.

The adapter exposes a small HTTP ingress for Vamos.  It intentionally builds
normal Hermes MessageEvents and delegates them to BasePlatformAdapter: Hermes
therefore owns session, background-task, and live-process state.  The adapter
never opens Hermes operational storage.

Configure under ``platforms.vamos.extra``.  ``host`` defaults to loopback;
set an explicit reverse-proxy/TLS endpoint before exposing it remotely.
"""

import asyncio
import hashlib
import hmac
import json
import logging
import re
import uuid
from typing import Any, Optional

from aiohttp import web
from gateway.config import Platform, PlatformConfig
from gateway.platforms.base import BasePlatformAdapter, MessageEvent, MessageType, SendResult
from gateway.session import SessionSource

from . import session_ingress_v1
from .conversation_identity import (
    ConversationIdentity,
    conversation_reference,
    validate_component,
    verified_identity,
)

logger = logging.getLogger(__name__)

_LOOPBACK_HOST = "127.0.0.1"
_DEFAULT_PORT = 8765
_PRESENTATION_TIMEOUT_SECONDS = 3.0
_PRESENTATION_SHUTDOWN_SECONDS = 1.0
_SETTLEMENT_MESSAGE_ID = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,127}\Z")


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
        self._conversation_associations: dict[str, ConversationIdentity] = {}
        self._presentation_tasks: set[asyncio.Task] = set()

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
        tasks = tuple(self._presentation_tasks)
        for task in tasks:
            task.cancel()
        if tasks:
            _, pending = await asyncio.wait(
                tasks, timeout=_PRESENTATION_SHUTDOWN_SECONDS,
            )
            if pending:
                logger.warning("[vamos] presentation tasks exceeded shutdown deadline")
        self._presentation_tasks.clear()
        if self._client is not None:
            await self._client.close()
            self._client = None
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

    def _bind_conversation(
        self, reference: str, identity: ConversationIdentity,
    ) -> ConversationIdentity:
        existing = self._conversation_associations.get(reference)
        if existing is not None and existing != identity:
            raise ValueError("conversation reference is already bound to another tuple")
        self._conversation_associations[reference] = identity
        return identity

    def _identity_for_reference(self, reference: str) -> ConversationIdentity:
        identity = self._conversation_associations.get(reference)
        if identity is None:
            raise ValueError("conversation reference has no active tuple association")
        if conversation_reference(identity.plan_dir, identity.thread_id) != reference:
            raise ValueError("conversation association conflicts with its reference")
        return identity

    async def _prompt(self, request: web.Request) -> web.Response:
        body = await self._body(request)
        try:
            command_id = validate_component(body.get("command_id"), "command_id")
            reference = body.get("conversation_reference")
            identity = verified_identity(body.get("plan_dir"), body.get("thread_id"), reference)
        except ValueError as exc:
            raise web.HTTPBadRequest(text=str(exc)) from exc
        owner = body.get("owner_email")
        prompt = body.get("prompt")
        if not isinstance(owner, str) or not owner.strip() or not isinstance(prompt, str) or not prompt.strip():
            raise web.HTTPBadRequest(text="owner_email and prompt are required")
        try:
            self._bind_conversation(reference, identity)
        except ValueError as exc:
            raise web.HTTPConflict(text=str(exc)) from exc
        event = MessageEvent(
            text=prompt,
            message_type=MessageType.TEXT,
            source=SessionSource(
                platform=Platform("vamos"), chat_id=reference, chat_name="Vamos",
                chat_type="thread", thread_id=reference, user_id=owner, user_name=owner,
            ),
            raw_message=body,
            message_id=command_id,
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

    async def _post_callback(
        self, identity: ConversationIdentity, event: dict[str, Any],
    ) -> None:
        import aiohttp
        if self._client is None:
            self._client = aiohttp.ClientSession()
        url = (
            f"{self._vamos_url}/agent-chat/api/hermes/threads/"
            f"{identity.thread_id}/events"
        )
        payload = {**event, "plan_dir": identity.plan_dir}
        async with self._client.post(
            url,
            json=payload,
            headers={"Authorization": f"Bearer {self._callback_token}"},
        ) as response:
            if response.status // 100 != 2:
                raise RuntimeError(f"Vamos callback: {response.status}")

    async def send(self, chat_id: str, content: str, reply_to: Optional[str] = None,
                   metadata: Optional[dict[str, Any]] = None) -> SendResult:
        """Deliver final Markdown through Vamos's authenticated callback."""
        try:
            identity = self._identity_for_reference(chat_id)
        except ValueError as exc:
            return SendResult(success=False, error=str(exc))
        event = {"id": uuid.uuid4().hex, "type": "final", "content": content}
        try:
            await self._post_callback(identity, event)
        except Exception as exc:
            return SendResult(success=False, error=str(exc))
        return SendResult(success=True, message_id=event["id"])

    def _capture_delivery_presentation(
        self, event: MessageEvent,
    ) -> tuple[ConversationIdentity, str, str, str] | None:
        source = event.source
        reference = getattr(source, "chat_id", None)
        if (
            getattr(source, "platform", None) != Platform("vamos")
            or not isinstance(reference, str)
            or getattr(source, "thread_id", None) != reference
        ):
            return None
        try:
            identity = self._identity_for_reference(reference)
        except ValueError as exc:
            logger.warning("[vamos] settlement presentation suppressed: %s", exc)
            return None
        metadata = getattr(event, "metadata", None)
        settlement_id = metadata.get("settlement_message_id") if isinstance(metadata, dict) else None
        if not isinstance(settlement_id, str) or not _SETTLEMENT_MESSAGE_ID.fullmatch(settlement_id):
            logger.warning("[vamos] settlement presentation suppressed: invalid settlement ID")
            return None
        if not isinstance(event.text, str):
            logger.warning("[vamos] settlement presentation suppressed: invalid wrapped turn")
            return None
        return identity, reference, settlement_id, event.text

    async def _present_settlement_delivery(
        self,
        identity: ConversationIdentity,
        reference: str,
        settlement_id: str,
        content: str,
    ) -> None:
        if conversation_reference(identity.plan_dir, identity.thread_id) != reference:
            raise ValueError("captured conversation association conflicts with its reference")
        event_id = hashlib.sha256(
            b"vamos-hermes-settlement-delivering-v1\x00" + settlement_id.encode("ascii")
        ).hexdigest()
        async with asyncio.timeout(_PRESENTATION_TIMEOUT_SECONDS):
            await self._post_callback(identity, {
                "id": event_id,
                "type": "settlement_delivering",
                "content": content,
            })

    def _presentation_done(self, task: asyncio.Task) -> None:
        self._presentation_tasks.discard(task)
        if task.cancelled():
            logger.warning("[vamos] settlement presentation cancelled")
            return
        exception = task.exception()
        if exception is not None:
            logger.warning("[vamos] settlement presentation failed: %s", exception)

    async def handle_admitted_next_turn(self, event: MessageEvent) -> None:
        normal_delivery = asyncio.create_task(super().handle_admitted_next_turn(event))
        presentation = self._capture_delivery_presentation(event)
        if presentation is not None:
            task = asyncio.create_task(self._present_settlement_delivery(*presentation))
            self._presentation_tasks.add(task)
            task.add_done_callback(self._presentation_done)
        return await normal_delivery

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
