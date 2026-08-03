import asyncio
from unittest.mock import patch

from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer

from gateway.config import GatewayConfig, Platform, PlatformConfig
from gateway.platform_registry import PlatformEntry, platform_registry
from gateway.platforms.base import MessageEvent
from gateway.run import GatewayRunner
from gateway.session import SessionSource, SessionStore, build_session_key
from gateway.session_admission import GatewayAdmissionPayload
from vamos_platform.adapter import VamosAdapter
from vamos_platform.conversation_identity import ConversationIdentity, conversation_reference


PLAN = "CoreyCole/plans/alpha"
THREAD = "0123456789abcdef0123456789abcdef"
REFERENCE = conversation_reference(PLAN, THREAD)


def plugin_adapter():
    platform_registry.register(PlatformEntry(
        name="vamos",
        label="Vamos",
        adapter_factory=lambda config: VamosAdapter(config),
        check_fn=lambda: True,
    ))
    adapter = VamosAdapter(PlatformConfig(extra={
        "token": "ingress-token",
        "callback_token": "callback-token",
        "vamos_url": "https://vamos.invalid",
        "typing_indicator": False,
    }))
    adapter.config.typing_indicator = False
    adapter._bind_conversation(REFERENCE, ConversationIdentity(PLAN, THREAD))
    return adapter


def real_runner(tmp_path, adapter):
    with patch("gateway.session.SessionStore._ensure_loaded"):
        store = SessionStore(tmp_path / "sessions", GatewayConfig())
    store._db = None
    store._loaded = True
    runner = object.__new__(GatewayRunner)
    runner.session_store = store
    runner.adapters = {Platform("vamos"): adapter}
    runner._profile_adapters = {}
    runner._exact_session_arbiters = {}
    runner._running = True
    runner._draining = False
    adapter.gateway_runner = runner
    return store, runner


def source():
    return SessionSource(
        platform=Platform("vamos"),
        chat_id=REFERENCE,
        thread_id=REFERENCE,
        chat_type="thread",
        user_id="owner@example.com",
        role_authorized=True,
    )


async def wait_for(predicate, timeout=2):
    async with asyncio.timeout(timeout):
        while not predicate():
            await asyncio.sleep(0)


async def test_real_h4_runner_revalidates_then_plugin_starts_real_lane_before_presentation(tmp_path):
    adapter = plugin_adapter()
    store, runner = real_runner(tmp_path, adapter)
    entry = store.get_or_create_session(source())
    handler_started = asyncio.Event()
    handler_release = asyncio.Event()
    callback_started = asyncio.Event()
    callback_release = asyncio.Event()
    callbacks = []

    async def handler(event):
        handler_started.set()
        await handler_release.wait()
        return ""

    async def callback(identity, event):
        callbacks.append((identity, event))
        callback_started.set()
        await callback_release.wait()

    adapter.set_message_handler(handler)
    adapter._post_callback = callback
    app = web.Application()
    app.router.add_post(
        "/vamos/session-ingress/v1/capabilities", adapter._session_ingress_capabilities
    )
    app.router.add_post("/vamos/session-ingress/v1/enqueue", adapter._session_ingress_enqueue)
    client = TestClient(TestServer(app))
    await client.start_server()
    try:
        auth = {"Authorization": "Bearer ingress-token"}
        capability = await client.post(
            "/vamos/session-ingress/v1/capabilities",
            json={"op": "capabilities", "version": 1},
            headers=auth,
        )
        assert capability.status == 200
        response = await client.post(
            "/vamos/session-ingress/v1/enqueue",
            json={
                "hermes_session_id": entry.session_id,
                "message": "immutable child response",
                "message_id": "settlement-1",
                "op": "enqueue",
                "pi_session_id": "pi:1.with-punctuation",
                "version": 1,
            },
            headers=auth,
        )
        assert response.status == 202
        assert await response.json() == {"code": "accepted_idle", "version": 1}
        await handler_started.wait()
        await callback_started.wait()
        key = build_session_key(source())
        assert key == f"agent:main:vamos:thread:{REFERENCE}:{REFERENCE}"
        assert key in adapter._active_sessions
        assert key in adapter._session_tasks
        handler_release.set()
        await wait_for(lambda: key not in adapter._active_sessions)
        assert len(callbacks) == 1
        identity, event = callbacks[0]
        assert identity == ConversationIdentity(PLAN, THREAD)
        assert event["type"] == "settlement_delivering"
        assert event["content"].endswith("immutable child response")
        callback_release.set()
        await wait_for(lambda: not adapter._presentation_tasks)
    finally:
        handler_release.set()
        callback_release.set()
        for arbiter in getattr(runner, "_exact_session_arbiters", {}).values():
            await arbiter.wait_idle()
        await client.close()
        await adapter.disconnect()


async def test_real_h4_stale_before_final_revalidation_never_invokes_plugin_delivery(tmp_path):
    adapter = plugin_adapter()
    store, runner = real_runner(tmp_path, adapter)
    entry = store.get_or_create_session(source())
    match = store.exact_session_matches(entry.session_id)[0]
    entry.revision += 1
    handler_calls = []
    callback_calls = []

    async def handler(event):
        handler_calls.append(event)

    async def callback(identity, event):
        callback_calls.append((identity, event))

    adapter.set_message_handler(handler)
    adapter._post_callback = callback
    await runner._deliver_exact_session_payload(GatewayAdmissionPayload(
        match=match,
        origin=match.origin,
        adapter=adapter,
        pi_session_id="pi-1",
        message_id="settlement-stale",
        text="opaque wrapper",
    ))
    await asyncio.sleep(0)
    assert handler_calls == []
    assert callback_calls == []
    assert adapter._presentation_tasks == set()
    await adapter.disconnect()


async def test_real_h4_super_preserves_handler_failure_cleanup_independent_of_presentation():
    adapter = plugin_adapter()
    callback_release = asyncio.Event()

    async def handler(event):
        raise RuntimeError("real H4 handler failure")

    async def callback(identity, event):
        if event["type"] == "settlement_delivering":
            await callback_release.wait()

    adapter.set_message_handler(handler)
    adapter._post_callback = callback
    event = MessageEvent(
        text="opaque wrapper",
        source=source(),
        internal=True,
        metadata={"settlement_message_id": "settlement-2", "pi_session_id": "pi.2"},
    )
    assert await adapter.handle_admitted_next_turn(event) is None
    key = build_session_key(source())
    await wait_for(lambda: key not in adapter._active_sessions)
    assert len(adapter._presentation_tasks) == 1
    callback_release.set()
    await wait_for(lambda: not adapter._presentation_tasks)
    await adapter.disconnect()


async def test_real_h4_external_cancellation_does_not_cross_cancel_presentation_or_corrupt_lane():
    adapter = plugin_adapter()
    handler_started = asyncio.Event()
    handler_release = asyncio.Event()
    callback_started = asyncio.Event()
    callback_release = asyncio.Event()

    async def handler(event):
        handler_started.set()
        await handler_release.wait()
        return ""

    async def callback(identity, event):
        callback_started.set()
        await callback_release.wait()

    adapter.set_message_handler(handler)
    adapter._post_callback = callback
    event = MessageEvent(
        text="opaque wrapper",
        source=source(),
        internal=True,
        metadata={"settlement_message_id": "settlement-3", "pi_session_id": "pi-3"},
    )
    admitted = asyncio.create_task(adapter.handle_admitted_next_turn(event))
    await handler_started.wait()
    await callback_started.wait()
    admitted.cancel()
    result = await asyncio.gather(admitted, return_exceptions=True)
    assert isinstance(result[0], asyncio.CancelledError)
    key = build_session_key(source())
    assert key in adapter._active_sessions
    assert len(adapter._presentation_tasks) == 1
    handler_release.set()
    await wait_for(lambda: key not in adapter._active_sessions)
    assert len(adapter._presentation_tasks) == 1
    callback_release.set()
    await wait_for(lambda: not adapter._presentation_tasks)
    await adapter.disconnect()
