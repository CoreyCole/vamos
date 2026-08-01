import asyncio
from unittest.mock import patch

from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer

from gateway.config import GatewayConfig, Platform, PlatformConfig
from gateway.platform_registry import PlatformEntry, platform_registry
from gateway.platforms.base import BasePlatformAdapter
from gateway.run import GatewayRunner
from gateway.session import SessionSource, SessionStore
from vamos_platform.adapter import VamosAdapter


class OriginatingAdapter(BasePlatformAdapter):
    supports_async_delivery = True

    def __init__(self):
        super().__init__(PlatformConfig(extra={}), Platform.MATRIX)
        self.events = []
        self.started = asyncio.Event()
        self.release = asyncio.Event()

    async def connect(self, *, is_reconnect=False):
        return True

    async def disconnect(self):
        return None

    async def send(self, chat_id, content, reply_to=None, metadata=None):
        raise AssertionError("exact-session admission is inbound")

    async def get_chat_info(self, chat_id):
        return {"name": "test", "type": "group"}

    async def handle_admitted_next_turn(self, event):
        self.events.append(event)
        self.started.set()
        await self.release.wait()


def real_runner(tmp_path):
    with patch("gateway.session.SessionStore._ensure_loaded"):
        store = SessionStore(tmp_path / "sessions", GatewayConfig())
    store._db = None
    store._loaded = True
    origin = OriginatingAdapter()
    runner = object.__new__(GatewayRunner)
    runner.session_store = store
    runner.adapters = {Platform.MATRIX: origin}
    runner._profile_adapters = {}
    runner._exact_session_arbiters = {}
    runner._running = True
    runner._draining = False
    return store, origin, runner


async def test_real_store_runner_and_originating_adapter_admit_without_waiting_for_completion(tmp_path):
    store, origin_adapter, runner = real_runner(tmp_path)
    source = SessionSource(
        platform=Platform.MATRIX,
        chat_id="!manager:example.org",
        chat_type="group",
        user_id="@manager:example.org",
        role_authorized=True,
    )
    entry = store.get_or_create_session(source)

    platform_registry.register(PlatformEntry(
        name="vamos",
        label="Vamos",
        adapter_factory=lambda config: VamosAdapter(config),
        check_fn=lambda: True,
    ))
    plugin = VamosAdapter(PlatformConfig(extra={
        "token": "ingress-token",
        "callback_token": "callback-token",
        "vamos_url": "https://vamos.invalid",
    }))
    plugin.gateway_runner = runner
    app = web.Application()
    app.router.add_post(
        "/vamos/session-ingress/v1/capabilities", plugin._session_ingress_capabilities
    )
    app.router.add_post("/vamos/session-ingress/v1/enqueue", plugin._session_ingress_enqueue)
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
        assert await capability.json() == {
            "capabilities": ["exact-session-next-turn-v1"],
            "code": "capabilities",
            "max_frame_bytes": 262144,
            "protocol_versions": [1],
            "version": 1,
        }

        response = await client.post(
            "/vamos/session-ingress/v1/enqueue",
            json={
                "hermes_session_id": entry.session_id,
                "message": "immutable child response",
                "message_id": "settlement-1",
                "op": "enqueue",
                "pi_session_id": "pi-1",
                "version": 1,
            },
            headers=auth,
        )
        assert response.status == 202
        assert await response.json() == {"code": "accepted_idle", "version": 1}
        await asyncio.wait_for(origin_adapter.started.wait(), timeout=1)
        assert len(origin_adapter.events) == 1
        event = origin_adapter.events[0]
        assert event.source == entry.origin
        assert event.source.platform == Platform.MATRIX
        assert event.metadata["settlement_message_id"] == "settlement-1"
        assert event.message_id is None
        assert event.reply_to_message_id is None
        assert event.text.endswith("immutable child response")
    finally:
        origin_adapter.release.set()
        arbiters = getattr(runner, "_exact_session_arbiters", {})
        for arbiter in arbiters.values():
            await arbiter.wait_idle()
        await client.close()
